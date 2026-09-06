# feat001 智能选岗参谋 — 技术设计文档

> 对应需求文档：[choose-job20260905.md](choose-job20260905.md)
> 相关文档：[测试问题记录](test-issues.md) · [后续迭代 TODO](iteration-todo.md)
> 本文档随开发过程同步更新（见 docs/rule.md 第 3 条）。

## 1. 技术选型

- 语言：Go 1.25
- Web 框架：Gin（`internal/api` 路由与接口层）
- Agent 编排：**v1 手写实现**（`internal/agent`，Supervisor + 各职能 Agent），不引入外部 Go Agent 框架
- LLM 客户端：`github.com/sashabaranov/go-openai`（OpenAI 兼容协议，**按能力分渠道接入**）
  - 文本：智谱 GLM-4-Flash（永久免费）；视觉：智谱 GLM-4V-Flash（永久免费，图片解析）—— 密钥 `LLM_API_KEY`
  - 向量化：硅基流动 BAAI/bge-m3（免费，1024 维）—— 密钥 `EMBEDDING_API_KEY`
  - 备选开源方案：PaddleOCR（Python 服务，中文表格效果最好）、Tesseract + gosseract（需本地安装原生引擎）、本地 Ollama（qwen2.5 / qwen2.5vl / bge-m3，改 base_url 即可）
  - 未配置密钥时：文本走关键词降级解析，图片返回 Mock need_confirm
- 缓存：`github.com/redis/go-redis/v9`（解析会话），连接失败自动降级进程内内存存储

## 2. 模块划分

### 2.1 多 Agent 运行时（去中心化总线架构）

v1 手写实现，不依赖外部 Agent 框架。**所有 Agent 是对等节点（Node），各自独立 goroutine 运行，通过共享消息总线协作**；层级调度、流水线是总线上的使用模式而非硬编码结构：

```plain
API 层（TraceID 中间件注入链路 ID）
  ↓ Runtime.Call（请求-响应，关联 ID 配对，带超时）
Supervisor（调度中枢，不持有 Agent 实例，只发消息）
  ↓ 消息总线 Bus（缓冲 channel，背压控制）
├── parser 节点（条件解析：文字/图片 → UserProfile）
├── assistant 节点（ReAct 循环：推理 ⇄ 工具调用）
└── 后续节点（researcher / analyzer / strategist / responder）
```

工程要点（对照多 Agent 协作实践）：

- **上下文隔离**：节点 Run 循环注入独立 agent context；长期记忆按 `agent_name` 命名空间隔离，不同 Agent 的记忆互不污染
- **记忆三层作用域**（`scope` 列）：`session` 会话消息（仅当前会话）、`user` 用户画像（跨会话沉淀，按 user_id 隔离）、`agent` Agent 经验洞察（跨会话/跨用户共享，新会话可直接复用，解决"换 session 就失忆"问题）；召回时三层合并按相似度统一排序
- **背压控制**：总线与节点 Inbox 均为带缓冲 channel + 非阻塞发送，缓冲满丢弃并计入 `Bus.Dropped()` 指标，绝不阻塞 Agent
- **优雅退出**：信号捕获 → HTTP 先停（处理完在途请求）→ Runtime 取消所有节点 context + WaitGroup 等待；节点内 panic 自动 recover，单个节点崩溃不影响整体
- **链路追踪**：API 入口生成 TraceID → 注入 context → 贯穿总线所有消息 → 响应头 `X-Trace-Id` 回写
- **请求-响应配对**：`Bus.Call` 按消息关联 ID + 来源节点双重匹配，防止请求自身被误配对

### 2.2 配置化装配（一切可变皆有默认）

`configs/config.yaml` 集中管理所有可变项，代码只提供骨架与默认值：

- **模型渠道表** `models`：OpenAI 兼容协议，key 即渠道名，Agent 按名引用；渠道级可配 `temperature`/`max_tokens`/`timeout`，单次调用还能临时覆盖 model/temperature（`llm.Options`）
- **Agent 定义表** `agents`：声明 `type`（parser/react，可扩展）、`model`、`prompt`、`tools`、`max_steps`、`temperature`；新增 Agent = 加配置
- **流程编排表** `flows`：流程名 → 节点名列表（如 `parse: [parser]`，未来 `advise: [parser, researcher, analyzer, strategist, responder]`），调整执行顺序/增删节点只改配置
- **Prompt 库** `internal/prompt`：**Prompt 是 Go 常量**（`prompts_*.go` 中按域定义，注册进 `Defaults` 表），**不绑定 Agent**——一个 Agent 可拥有多个变体（如 `assistant`/`assistant_beginner`/`assistant_advanced`），运行时按场景选用；支持 text/template 变量渲染（如 `{{.Mode}}`）
- **ServiceContext** `internal/svc`：所有依赖（配置/模型/存储/工具/运行时）在 `NewServiceContext` 统一装配一次，各 Handler/Agent 从 svc 取用，新增依赖只需加一个字段（go-zero 风格）
- **运行时参数** `runtime`：总线缓冲、节点收件箱、调用超时均可调
- **工具注册表** `internal/tool`：实现 `Tool` 接口并注册，ReAct Agent 通过 function calling 调用

### 2.3 模块清单

| 模块 | 文件 | 职责 |
| --- | --- | --- |
| 消息总线 | `internal/runtime/bus.go` | Message 协议、Publish（背压）、Call（请求-响应配对） |
| 节点 | `internal/runtime/node.go` | 独立 Run 循环、panic recover、上下文隔离 |
| 运行时 | `internal/runtime/runtime.go` | 节点生命周期、消息分发、优雅退出 |
| 流程编排 | `internal/runtime/flow.go` | 按配置 flows 表顺序驱动节点（流水线模式） |
| 节点装配 | `internal/agent/nodes.go` | 按配置把 parser/react 包装为总线节点 |
| Supervisor | `internal/agent/supervisor.go` | 调度中枢：经流程编排器驱动节点 |
| Parser Agent | `internal/agent/parser.go` | 条件解析：文字/图片 → UserProfile，置信度判定 |
| ReAct Agent | `internal/agent/react.go` | 手写推理-行动循环，步数上限防死循环 |
| Memory | `internal/agent/memory.go` | 长期记忆：三层作用域写入/召回 |
| 降级解析 | `internal/agent/simple_parser.go` | 无 LLM 时的关键词/正则规则解析 |
| 数据模型 | `internal/types` | 跨层共享模型（UserProfile/ParseResult），避免循环依赖 |
| 模型渠道 | `internal/llm` | 多渠道管理器：文本/视觉/向量/function calling/参数覆盖 |
| Prompt 库 | `internal/prompt` | 模板库：Go 常量注册表（Defaults）、变量渲染、多变体按名取用 |
| 服务上下文 | `internal/svc` | ServiceContext：所有依赖统一装配与载体（go-zero 风格） |
| 工具 | `internal/tool` | 工具注册表 + 示例工具 query_positions |
| 存储 | `internal/store` | MySQL（用户/收藏）、pgvector（记忆）、Redis（通用缓存预留） |
| 业务层 | `internal/api/logic` | 业务逻辑（user/favorite/parse），不感知 HTTP、不写 SQL，错误用哨兵 error 表达 |
| 接口层 | `internal/api` | 纯 HTTP 协议层：路由注册、参数校验、调用 logic、响应封装（不触碰存储） |

## 3. 接口设计

| 接口 | 方法 | 说明 | 状态 |
| --- | --- | --- | --- |
| `/health` | GET | 健康检查（公开） | 已实现 |
| `/auth/register` | POST | 用户注册（bcrypt 密码存储） | 已实现 |
| `/auth/login` | POST | 用户登录，返回 JWT | 已实现 |
| `/api/v1/parse` | POST | 条件解析（文本 + 毕业证图片多源融合），低置信度返回 need_confirm；快照落 MySQL | 已实现（JWT） |
| `/api/v1/parse/confirm` | POST | 确认修正低置信度字段，合并返回完整 profile | 已实现（JWT） |
| `/api/v1/profile` | GET | 用户条件档案（最近一次解析快照，解析页直接展示） | 已实现（JWT） |
| `/api/v1/positions` | GET | 岗位分页查询（考试类型/省份/关键词/学历/专业大类/政治面貌/应届过滤） | 已实现（JWT） |
| `/api/v1/favorites` | POST/GET | 收藏/取消收藏岗位、收藏列表（按登录用户隔离） | 已实现（JWT） |
| `/api/v1/chat` | POST | 多轮追问（将基于 pgvector 记忆召回 + RAG） | 占位（501） |
| `/api/v1/advise` | POST | 选岗推荐（冲稳保） | 待实现 |
| `/api/v1/reports/{id}` | GET | 报告查询 | 待实现 |

### 关键流程

- **解析（成功路径）**：API → Supervisor → Parser Agent → LLM（PRD 6.2 Prompt）→ JSON 抽取 → 置信度判定 → success
- **解析（需确认）**：任一字段置信度 < 0.85 → need_confirm，会话存入 Redis（TTL 30min）→ 用户调用 confirm 合并字段 → success（置信度视为 1.0）
- **降级**：未配置 LLM 密钥时，文本走关键词规则解析，图片返回 Mock need_confirm

## 4. 数据设计（存储分层）

| 存储 | 承载数据 | 说明 |
| --- | --- | --- |
| **MySQL**（Docker `kaogong-mysql`） | `users`（用户名 + bcrypt 密码哈希）、`user_sessions`（用户条件快照 + 会话状态）、`favorites`（收藏，user_id+position_id 唯一） | 结构化业务数据，用户体系为注册账号（userId） |
| **PostgreSQL + pgvector**（Docker `kaogong-pgvector`） | `memories`（文本 + embedding + scope） | Agent 长期记忆 + RAG 知识库（PRD 5.1 的 positions/position_embeddings 随后续迭代落地） |

分层原则：会话消息等"长期记忆"向量化存 pgvector 供语义召回，不存 MySQL；MySQL 只存结构化业务数据。外部存储不可用时降级跳过持久化，不阻塞启动。

> 注：OCR 确认流程曾用 Redis 暂存待确认数据，后改为**无状态设计**（见 4.2）。Redis 以通用缓存客户端形式保留在 svc（`svc.Redis`）备用，预留给后续业务（对话热点上下文、查询结果缓存、LLM 结果缓存等），当前无业务依赖，不可用时仅告警不影响运行。

### 4.2 OCR 确认流程（无状态设计）

与 PRD 3.2.2 的 session_id 暂存方案不同，本项目采用无状态设计：

- `/parse` 返回 `need_confirm` 时，`profile_partial` 与 `uncertain_fields` 全部随响应下发，客户端持有
- `/parse/confirm` 请求由客户端原样带回 `profile_partial` + 用户确认字段，服务端合并落库
- 理由：待确认数据本就是用户自己的数据，无防篡改需求；无状态更简单、天然支持水平扩展，少维护一个 Redis 依赖

### 4.3 多源融合解析

`/parse` 支持文本 + 多张图片一次提交（`content` + `images[]`），融合出一份画像：

- **并行执行**：文本与各图片的解析互不依赖，errgroup 并行发起 LLM 调用，总耗时≈最慢一路
- **方向约束**：文本与毕业证同属"用户条件"方向；职位表截图属于"岗位要求"方向，已从解析页移除，待选岗分析链路实现
- **融合规则**（代码层，`agent/merge.go`）：证件优先（学历/专业）、冲突检测进确认、其余字段互补、省份并集、应届三态合并
- 融合结果统一过规则校验层（`validateResult`）；冲突条目（带建议值）即使字段有值也保留确认
- **LLM 输出容错**：小模型输出格式不稳定（confidence 可能是字符串、uncertain_fields 可能是对象），统一 RawMessage 接收后容错解析（`parseConfidence` / `parseUncertainFields`）

### 4.1 用户与鉴权

- 注册 `POST /auth/register`、登录 `POST /auth/login`（返回 JWT，HS256，默认 72h）
- 密钥：`jwt.secret` 配置或环境变量 `JWT_SECRET`；未配置时启动生成随机密钥（重启失效，仅开发用）
- `/api/v1/*` 全部走 JWT 中间件，用户身份从 token 的 `user_id` claim 获取，接口不再接收 device_id

- **向量化**：`llm.Manager.Embed`（默认硅基流动 bge-m3，1024 维；维度在 `llm.channels.embedding.dim` 配置，与向量库表结构不一致时自动重建 memories 表）
- **记忆召回**：`agent.Memory.Recall` 按余弦距离召回会话内相关历史，供追问时构造上下文
- OCR 当前为视觉大模型方案；未配置密钥时文本走关键词降级解析，图片返回 Mock need_confirm

## 5. Prompt 管理

Prompt 是代码资产，以 **Go 常量**集中定义在 `internal/prompt/prompts_*.go`（按业务域分文件），注册进 `prompt.Defaults` 表：

- **多变体**：一个 Agent 可拥有多个 Prompt 常量（如 `PromptAssistant` / `PromptAssistantBeginner` / `PromptAssistantAdvanced`、文本解析 `PromptParser` / 证件解析 `PromptParserDiploma`），运行时按场景（模式/输入类型）选用，`agents.*.prompt` 只是默认名
- **模板变量**：支持 text/template（如 `{{.Mode}}`、`{{.Kind}}`），按场景动态渲染
- **命名规范**：Prompt 名常量在 `internal/prompt/names.go` 集中定义（`NameXxx`），`defaults.go` 按常量注册——禁止在业务代码里写字面量 key
- **新增 Prompt**：在对应 `prompts_*.go` 中定义常量 → `names.go` 定义名字 → `defaults.go` 登记

当前内置：`parser`、`parser_diploma`、`parser_image_user`、`assistant`、`assistant_beginner`、`assistant_advanced`。

## 6. 常量约定

为可查找性，以下 key 一律使用常量，禁止魔法字符串：

- Prompt 名：`internal/prompt/names.go`（`prompt.NameXxx`）
- 模型渠道名：`internal/llm`（`llm.ChannelChat` 等）
- Agent 类型：`internal/agent`（`agent.AgentTypeParser` / `AgentTypeReact`）
- 流程名：`internal/agent/supervisor.go`（`agent.FlowParse`）
- UserProfile 字段名：`internal/types`（`types.FieldXxx`，用于 uncertain/confirmed 字段 key）
- 图片类型：`internal/types`（`types.ImageTypeDiploma` / `ImageTypePosition`）
