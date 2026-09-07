# 公考参谋部（ai-start）

基于多 Agent 协作的智能公考选岗分析系统（纯后端 API 服务）。详见 [产品需求 PRD](docs/feat001/choose-job20260905.md)。

## 架构理念

本项目采用企业级 AI Agent 的主流落地形态——**确定性工作流为骨架，动态决策只放在关键节点**：

```plain
确定性骨架（flows 配置编排，流程可审计、可复现）   ← 主干，必须可靠
  └── 动态决策点（LLM 意图路由、工具选择）          ← 只在岔路口让模型选
       └── 完全自主段（ReAct 推理-行动循环）        ← 只在沙盒化的小范围任务
```

依据：Anthropic《Building Effective Agents》主张"大多数场景用最简单的方案，路径无法预知时才上自主 Agent"；阿里云百炼、腾讯云 ADP 等企业平台均以"工作流为骨架、Agent 为节点"为主推落地方式；纯自主 Agent 在生产环境存在幻觉不可控、成本不可预测、无法审计追责等问题。

因此本项目不追求"全自动 Agent"，而是：流程可审计（flows 配置化）+ 决策点 agentic（ReAct 工具选择、LLM 意图识别）+ 失败可降级（置信度确认、关键词兜底）。

## AI 能力边界划分原则

**核心判据：程序能 100% 确定的，不问 AI；程序解析不了的（模糊、开放、需要理解意图），才交给 AI。**

| 层 | 适用条件 | 本项目例子 |
| --- | --- | --- |
| 代码规则 | 逻辑可枚举、要求 100% 可靠、可单测 | 学历高低判断、专业大类映射、置信度阈值判定 |
| 第三方库/服务 | 有成熟方案、输入输出结构化 | go-openai、pgx、bcrypt、jwt |
| AI 直接处理 | 输入非结构化、但每次都要做、参数固定 | 自然语言→UserProfile、图片→结构化字段 |
| 暴露为 AI 工具 | **模型需要自主决定"要不要调、调哪个、参数是什么"** | ReAct 中的 `query_positions` |

补充原则：

- **"能确定"包含"要求可靠"**：错了代价高的逻辑（如专业匹配，错了用户就报错岗位）必须留在代码规则里，哪怕写起来麻烦——PRD 第八章明确要求"专业匹配走规则引擎，LLM 只负责解释，不参与判定"
- **AI 内部再分一层**：每次都要做、参数固定的 → AI 直接处理（如 parse）；要不要做取决于上下文的 → 才注册为工具让模型自选（工具暴露有成本：多轮推理、token 消耗、选错风险、Schema 占上下文）
- **边界是动态的**：随着对业务和模型的了解加深，代码规则和 AI 能力之间会互相迁移（稳定的 AI 能力可固化成规则，规则覆盖不住的模糊场景可下沉给 AI）——这就是配置化（模型可换、Prompt 可变）的价值

新增能力时的自查清单：**输入结构化吗？→ 调用时机固定吗？→ 结果必须 100% 可靠吗？→ 需要模型决定调不调吗？**

## 目录结构

```plain
├── cmd/
│   └── server/        # 服务启动入口
├── internal/          # 内部业务代码（不对外暴露）
│   ├── agent/         # 业务 Agent：parser / assistant(ReAct) / memory / supervisor
│   ├── runtime/       # Agent 运行时基建（业务无关）：总线/节点/生命周期/流程编排
│   ├── api/           # HTTP 接口层：只做路由/鉴权/参数校验/响应封装（不含业务逻辑）
│   │   └── logic/     # 业务逻辑层：user / favorite / parse（不感知 HTTP，不写 SQL）
│   ├── llm/           # 模型渠道管理器（OpenAI 兼容：文本/视觉/向量/工具调用/参数覆盖）
│   ├── types/         # 跨层共享数据模型
│   ├── prompt/        # Prompt 模板库（Go 常量定义，多变体按名取用，模板渲染）
│   ├── store/         # 存储层：MySQL（用户/收藏）+ pgvector（记忆）+ Redis（会话缓存）
│   ├── svc/           # ServiceContext：统一依赖装配与注入（go-zero 风格）
│   └── tool/          # 工具注册表（Agent function calling 可调用的工具）
├── web/
│   └── index.html       # 前端单页应用（登录/解析/确认/收藏/对话，Gin 托管，随需求迭代）
├── configs/           # 配置目录（config.go 加载代码 + config.yaml 主配置）
│   #                  # 主配置含：模型渠道表 + Agent 定义表 + 流程编排表
└── docs/              # 项目文档
    ├── rule.md        # 开发规范
    ├── feat001/       # 条件解析/岗位查询（PRD + 技术文档）
    ├── feat002/       # 多轮追问多 Agent 对话中枢（PRD + 技术文档）
    └── feat003/       # 多 Agent 记忆体系（PRD + 技术文档）
```

## 技术选型

- Web 框架：Gin
- LLM：`github.com/sashabaranov/go-openai`（OpenAI 兼容协议，按能力分渠道接入）
  - 文本：智谱 GLM-4-Flash（永久免费），环境变量 `LLM_API_KEY`
  - 视觉：智谱 GLM-4V-Flash（永久免费，图片识别），复用 `LLM_API_KEY`
  - 向量化：硅基流动 BAAI/bge-m3（免费，1024 维），环境变量 `EMBEDDING_API_KEY`
  - 可无缝替换为 OpenAI / DeepSeek / 本地 Ollama 开源模型，只改配置
- 存储分层：MySQL（用户/收藏等结构化数据，GORM）、PostgreSQL+pgvector（长期记忆/RAG 向量检索，pgx）、Redis（短期会话缓存，go-redis）

## 存储选型问答

**Q：会话历史/用户档案为什么存 MySQL，向量数据库不是更好吗？**

A：向量数据库只擅长一件事——按语义相似度找内容，它不是"更好的数据库"。选存储看查询模式：

- **会话历史/档案是精确查询**："取我最新的档案"、"这条会话确认了吗"——需要精确匹配、状态过滤、排序、事务一致性，这是关系型数据库的主场
- **向量库回答的是另一类问题**："和这句话意思相近的历史经验有哪些"——输入一段话，输出语义最像的 N 条，做不了也不想精确查询
- **向量是派生数据**：embedding 由模型算出，换模型（维度变了）就要全量重算——向量库存的是"为检索而生的副本"，不是权威数据源（source of truth）。权威数据必须在 MySQL，向量库挂了可重建

分工配合：MySQL 回答"是什么、是谁的、什么状态"；pgvector 回答"和这句话意思相近的有哪些"。chat 追问时两者配合：MySQL 拿档案（精确）+ pgvector 召回记忆（语义），一起喂给 LLM。

## 多 Agent 设计问答

**Q：多 Agent 怎么划分？它们能用同一个模型吗？**

A：按业务职责划分（像团队分工），一个 Agent 只干一件事。划分信号：职责单一、输入输出契约清晰、Prompt/工具/模型需求不同。

**Agent ≠ 模型**：`Agent = 角色（Prompt）+ 工具集 + 模型渠道 + 记忆空间`。模型只是零件，多个 Agent 可共用同一渠道（无状态、安全），也可按任务难度分配不同模型（解析用小模型、策略用旗舰模型）——渠道与 Agent 两张表按名引用，改配置即可。注意区分：模型可共享，记忆必须按 `agent_name` 隔离。

**Q：既然能共用模型，为什么不直接用单一 Agent 做全部？多 Agent 的意义是什么？**

A：多 Agent 的价值不在功能，在质量和工程：

1. **Prompt 容量与角色冲突**：单 Agent 要把"严谨抽取""规则匹配""生动表达"写进一个 Prompt，指令互相冲突，模型角色越多演得越差；拆开后每个 Prompt 短而专注
2. **质量可定位可回归**：每步输出有契约（Parser 出 UserProfile、Analyzer 出竞争分），可逐个环节评测调优
3. **参数按需分配**：Parser 要 temperature 0.1 严谨、Responder 要 0.7 生动，单 Agent 只能一套参数
4. **故障隔离**：单个节点崩溃不影响整体（总线架构下节点 panic 自动恢复）

代价也要认：串行链路延迟叠加、编排复杂度、节点间信息损耗。

判断标准：**任务能拆成"要求互相冲突的角色"时值得多 Agent；拆出来角色都差不多，单 Agent 就够。**

**Q：一个 Agent 里有多次 LLM 调用（如 Parser 的文本解析和图片解析），算多个 Agent 吗？**

A：不算。**拆 Agent 看职责和契约，不看调用次数**。文本解析和图片解析共用同一系统 Prompt、同一输出契约（UserProfile）、同一校验层——它们是同一职责下的两个输入通道，如同一个翻译官处理文字和图片两种载体。ReAct Agent 循环里调 N 次模型也不算 N 个 Agent。

真正该拆的信号：Prompt 不同、输出契约不同（如图片解析产出岗位而非用户条件）、需要独立失败语义、工具集不同。

如果只是为了延迟优化（文本/图片解析并行），那是流程层的事（flow 并行组），不是拆 Agent。

**Q：企业异步任务是不是用 RabbitMQ？我们的 Bus 总线和它有什么区别？**

A：企业是**两层并存，各司其职**。RabbitMQ/Kafka 在基础设施层（服务间、跨进程、持久化、ACK/重试/死信），Agent 总线在应用层（Agent 间协作、进程内、带会话上下文）。

去中心化总线的设计目的不是耗时异步，而是 **Agent 自治**：独立运行、独立崩溃恢复、自由对话、注册即接入、记忆/工具隔离——异步能力只是副产品。

类比：RabbitMQ 是城市间的邮政系统，Bus 是办公室里同事直接喊话。不会用对讲机寄跨城市的信，也不会让同事间写信走邮局。

| 能力 | 本项目 Bus | RabbitMQ |
| --- | --- | --- |
| 进程内/跨进程 | 进程内 | 跨进程跨机器 |
| 消息持久化 | 重启即丢 | 落盘 |
| 确认/重试/死信 | 无 | ACK + 重试 + DLQ |
| 路由规则 | To/广播 | exchange + routing key |
| 消费组负载均衡 | 单节点单消费 | 多消费者竞争消费 |
| 请求-响应配对 | 原生支持 | 需自建 correlation ID |

若要对标企业两层架构：耗时任务（如报告生成）由 RabbitMQ 做可靠分发，Agent 协作仍走内部 Bus——`chat 触发 → 任务落库 + MQ 消息 → worker 消费执行 Agent 流程（内部 Bus 协作）→ 完成回写`。当前单实例用 Bus 足够，需要时再引入。

## 流式与事件机制问答

**Q：SSE 的 sink 在 API 层创建，Agent 节点在另一个 goroutine，它怎么拿到的？**

A：sink 没有穿越总线（函数传不进 channel），是**双 sink + 消息 ID 接力**：

```plain
[API 层] 创建真 sink（写 SSE）→ 登记进总线 pending 表（msgID → sink）
[总线]   消息只带数据（ID/UserID/内容）传给节点
[节点]   Node.Run 给每条消息注入"转发 sink"：事件 → bus.EmitToMessage(msgID, ev)
[ReAct]  EmitEvent → 转发 sink → 总线按 msgID 查表 → 执行真 sink 写 SSE
```

即：**注册在总线，执行在节点 goroutine**。总线只是注册表，不做调度。与身份（UserID）、TraceID 同一模式：能序列化的放消息字段，不能序列化的（函数）用注册表 + ID 关联。

注意边界：当前单节点调用安全（handler 阻塞等待时只有节点在写）；将来 flow 并行组多节点共享同一 sink 并发写 SSE 时需要加锁串行化。

**Q：SSE 连接什么时候关闭？**

A：流的寿命 = 一次回答的生成时长，四种关闭时机：

1. **正常完成**：写完 `done` 事件，handler 返回，连接关闭（响应头的 `keep-alive` 是 TCP 连接复用，不是流常驻）
2. **出错**：写 `error` 事件后关闭
3. **客户端断开**：用户关页面 → 请求 ctx 取消 → 总线 Call 的 `ctx.Done()` 分支返回 → 关闭，无 goroutine 泄漏
4. **超时兜底**：节点调用超过 `call_timeout`（默认 30s）→ 兜底文案 → done → 关闭

待优化项：无心跳机制，模型长时间无输出时可能被代理判闲置断连（后续可加 15s `: ping` 心跳）。

**Q：多 Agent 的记忆怎么划分才合理？（feat003）**

A：按"内容本质属于谁"划分权责，而不是按存储技术一刀切：

| 内容 | 存哪 | 作用域 | 共享性 |
| --- | --- | --- | --- |
| 原始对话消息 | Redis 短期缓冲 | session | 编排层统一管（20 条/2h） |
| 用户画像事实 | pgvector | user | 跨 Agent 共享（选岗参谋和面试考官都需要知道用户是谁） |
| Agent 经验洞察 | pgvector | agent | 按 Agent 隔离（选岗经验对面试考官是噪音） |

两个关键原则：

1. **召回在 Agent 内部**：路由到谁，谁在执行开头召回自己的经验——不在编排层路由前统一召回（否则写死 Agent 名会张冠李戴）
2. **不是什么消息都值得长期记**：原始闲聊不进向量库（噪音淹没 + embedding 成本）；长期记忆只存"有价值的内容"——规则触发（档案变更/收藏）或 Agent 自主沉淀（`save_insight` 工具，白名单控制）

一句话：**编排层管"对话流水"，Agent 管"自己的心得"，画像大家共享**。

**Q：企业级多 Agent 记忆也是这样吗？每轮提问都召回一次，合理吗？**

A：分层结构与企业同构（Working/Episodic/Semantic Memory ≈ 我们的 Redis 短期 / 会话 / 画像+经验向量库），**每轮召回也是 RAG 式对话的标准做法**，但企业级会叠加几层精细化控制：

| 能力 | 本项目 | 企业级 |
| --- | --- | --- |
| 分层记忆 | ✅ 三层 | ✅ 同构 |
| 每轮召回 | ✅ 有 | ✅ 有，但带**召回闸门**（承接上文的短追问跳过向量召回） |
| 会话压缩摘要 | ❌ Redis 硬截断 20 条 | ✅ 旧消息压缩成摘要沉淀长期 |
| 记忆衰减/时效评分 | ❌ 纯相似度排序 | ✅ 相似度 × 新鲜度综合排序 |
| embedding 批量化 | ❌ 逐条即时 | ✅ 队列异步批量 |

差距即 TODO：召回闸门、会话压缩（与 save_insight 配合：会话结束沉淀摘要）、记忆衰减评分。

## 开发规范

见 [docs/rule.md](docs/rule.md)。要点：代码必加注释；每个需求在 `docs/` 下建有对应目录，包含需求文档与技术文档，技术文档随开发同步更新。

## 依赖环境

本地依赖通过 Docker 启动（全部不可用时代码自动降级，不阻塞启动）：

```bash
docker start kaogong-mysql kaogong-pgvector kaogong-redis
```

- MySQL（3306）：用户、会话、收藏等结构化业务数据
- PostgreSQL + pgvector（5432）：Agent 长期记忆、RAG 知识库（向量检索）
- Redis（6379）：通用缓存客户端已接入 svc（预留给后续业务，当前无依赖）

## 运行

```bash
# 首次：复制配置模板（config.yaml 含密钥，已 gitignore 不入库）
cp configs/config.example.yaml configs/config.yaml
# 编辑 config.yaml 填入密钥，或用环境变量注入：
export LLM_API_KEY=智谱key        # https://open.bigmodel.cn 注册
export EMBEDDING_API_KEY=硅基流动key # https://cloud.siliconflow.cn 注册

go run ./cmd/server
```

启动后浏览器访问 http://localhost:8080 即可使用前端页面（登录/注册 → 条件解析 → 确认修正 → 收藏 → 多轮追问）。

## 职位表数据导入

历年职位表为官方 Excel（国考：bm.scs.gov.cn 年度考录专题；省考：各省人事考试网），用独立脚本导入（不影响主服务）：

```bash
# 预览解析结果（不入库）
go run ./scripts/import_positions --file 职位表.xlsx --exam-type 国考 --year 2025 --province 国家 --dry-run

# 正式导入
go run ./scripts/import_positions --file 职位表.xlsx --exam-type 省考 --year 2025 --province 广东
```

表头按关键词包含匹配，兼容国考/省考常见变体（部门名称/用人司局/招录单位等）。

启动后可用接口：

- `POST /auth/register`：用户注册（username + password，密码 bcrypt 存储）
- `POST /auth/login`：用户登录，返回 JWT
- 以下接口均需请求头 `Authorization: Bearer <token>`：
  - `GET /health`：健康检查（公开）
  - `POST /api/v1/parse`：条件解析（文字/图片 → 结构化条件，低置信度返回 need_confirm）
  - `POST /api/v1/parse/confirm`：OCR 低置信度字段确认修正
  - `POST /api/v1/favorites` / `GET /api/v1/favorites`：收藏/取消收藏/收藏列表（按登录用户隔离）
  - `POST /api/v1/chat`：多轮追问（P1 占位，后续迭代接入）

## 核心链路速查（feat004 选岗推荐）

### advise 执行链（从用户说话到报告落库）

```plain
用户："帮我生成选岗报告"
  → internal/api/chat.go（SSE 入口）
  → internal/api/logic/chat.go（ChatService：短期记忆 + 画像 + 路由）
  → router 节点（意图分类）→ advisor
  → internal/agent/react.go（advisor ReAct 循环）→ 调 run_advise 工具
  → internal/tool/advise.go → svc 闭包注入
  → internal/api/logic/advise.go（AdviseService.Run：建档 → 执行流程 → 落库）
  → flows.advise 流程（configs/config.yaml 配置节点顺序）
      researcher（纯代码 SQL 检索）→ analyzer（LLM 竞争分析）→ strategist（LLM 冲稳保）→ responder（LLM 报告）
  → internal/store/report.go（reports 表落库）→ SSE 回显
```

### 代码速查表

| 环节 | 位置 |
| --- | --- |
| 触发工具 | `internal/tool/advise.go` |
| 业务编排 | `internal/api/logic/advise.go` |
| 流程配置 | `configs/config.yaml` flows 段 |
| 节点装配 | `internal/agent/nodes.go`（BuildRuntime） |
| 节点实现 | `internal/agent/advise.go` |
| Prompt | `internal/prompt/prompts_advise.go` |
| 报告存储 | `internal/store/report.go` |
| 报告接口 | `internal/api/report.go` |

### Q&A

**Q：router 是 ReAct 吗？**

A：不是，是**两级决策**：router 是一次性 LLM 分类（无循环无工具，低温求稳）；目标 Agent（如 advisor）才是 ReAct（思考→调工具→再思考）。advise 流水线内部的四个节点也不是 ReAct——`llm_step` 单次调用 + `code` 纯代码节点。三种形态：分类（router）、推理循环（react）、固定步骤（llm_step/code）。

**Q：ReAct 和 advisor 是什么关系？**

A：ReAct 是**类型/工作方式**（模具），advisor 是**具体 Agent**（实例）。配置里 `advisor: {type: react}`——同一类型可有多个实例（advisor、assistant），各自独立的 Prompt/工具白名单/温度/记忆，注册为总线上按名字路由的独立节点。新增 ReAct Agent = 配置表加一条新名字。

**Q：advisor 的步骤流（researcher 等）在哪？**

A：不在 advisor 里面。它们是总线上的平等节点，由 `flows.advise` 配置串成流水线，advisor 只通过 `run_advise` 工具触发。两层分工：ReAct 决定"做不做"（决策层），flow 保证"怎么做"（执行层确定性）。

**Q：advisor 如何感知和调用工具？**

A：四步——①感知：白名单工具定义（名称/描述/参数 Schema）随请求发给模型；②决策：模型回复 tool_calls（自己选工具填参数）；③执行：ReAct 循环经工具注册表按名执行（白名单校验 + JWT 身份校验）；④反馈：结果回填消息再调模型，得到基于真实数据的回答。工具实现在 `internal/tool/`（一文件一工具），业务逻辑在 logic 层（工具是薄壳，svc 装配时闭包注入）。

**Q：是不是只有 ReAct 能多轮对话？工具可以是什么？**

A：两个维度别混——ReAct 管"单轮内的推理深度"，多轮对话管"跨轮的记忆"。本项目里对话入口的 Agent 用 ReAct（需要自主决策调不调工具），流水线节点用 llm_step/code（执行确定性任务）。

工具是抽象能力，里面可以装任何东西：简单函数/SQL（query_positions）、内部业务服务（update_profile）、**另一个 Agent 的流程**（run_advise 就是包了四个节点的流水线）、第三方 SDK、外部 HTTP API。这是"Agents as Tools"模式——上层 Agent 把下游整条子链路当作一个能力调用，内部随便改，上层无感知，系统因此成为可嵌套的积木。

**Q：llm_step 算是 Agent 吗？**

A：严格说不算——它是"套了节点外壳的一次 LLM 调用"。**节点是运行时单元，Agent 是行为属性**（核心特征是自主决策）。自主程度分级：`code`（0 决策）< `llm_step`（固定输出）< `router`（单次判断）< `react`（循环决策）。

统一包装成节点的收益：寻址统一（flows 不关心下游是 LLM 还是代码）、生命周期统一（启动/停止/panic 恢复）、可观测统一（TraceID/事件流）、可替换（researcher 想升级智能只改配置 `type: code→react`）。这与企业级 workflow 引擎（Dify 节点、LangGraph node）同思路：统一节点抽象，内部自由度不同。

**Q：项目里所有 Agent 在哪里定义？怎么运行的？**

A：全部声明在 `configs/config.yaml` 的 `agents` 表（名字 + 类型 + 模型渠道 + Prompt + 工具白名单 + 温度），新增 Agent = 加一行配置。

运行时：`BuildRuntime` 按表装配 → 每个节点注册到总线 → `Runtime.Start` 给每个节点起一个 goroutine 跑 `Run` 循环（阻塞在各自 Inbox 上等消息，不占 CPU）→ 另有 1 个总线分发协程。启动日志可见节点总数（如 `count=8`）。

闭环：**配置声明 → 装配成节点 → 各自协程运行 → 总线通信**。

## 知识库与记忆问答

**Q：知识库是什么？就是 RAG 记忆存储吗？**

A：不是。两者是"内容从哪来"的区别：

- **记忆（Memory）**：系统"经历过的"（对话记录、用户画像、Agent 经验）——因用户而异、动态产生，回答"我记得你"
- **知识库（Knowledge Base）**：系统"学习过的"领域知识（政策文件、报考指南、专业目录）——公共的、预先灌入的，回答"我知道这个知识"
- **RAG 不是存储，是检索方式**（检索增强生成）：记忆和知识库都可以走向量检索，所以容易混

本项目现状：pgvector 里只有记忆（对话/画像/经验），知识库内容未灌入（TODO）；岗位查询走 SQL 精确过滤而非向量检索。

**Q：知识库要分块，记忆也要分块吗？**

A：分块不是记忆/知识库的固有属性，是**内容长度决定的技术手段**——长内容（政策文件、Excel 职位表）必须分块，短内容（对话消息、经验洞察）不用。本项目记忆不分块是因为存的都是短文本，与它是"记忆"无关。

**Q：有了知识库后，检索是不是记忆+知识库一起查？**

A：按成本分层，不是一刀切：

| 内容 | 检索策略 |
| --- | --- |
| 短期记忆（Redis 最近对话） | 每轮必带（便宜，连贯性基础） |
| 长期记忆（pgvector 画像/经验） | 按需召回（召回闸门，短追问跳过） |
| 知识库（政策/指南） | 工具化按需（注册 `search_knowledge` 工具，模型自己判断要不要查） |

原则：**便宜的必带，贵的都按需**——知识库不做"每次都查"，做成工具挂白名单，由模型按问题决定。这也符合"AI 能力边界划分原则"（要不要查是模型的判断，怎么查是代码的事）。

**Q：长期记忆的召回闸门怎么决策？**

A：闸门回答的问题是"这轮提问值不值得去向量库翻旧账"。三种方式从便宜到贵：

1. **规则判断**（零成本，先做）：跳过信号——消息很短（<10 字）、含指代词（"这个/那个/刚才/继续"）、纯闲聊；触发信号——时间跨度词（"之前/上次/上周"）、历史指向（"我收藏过的"）
2. **LLM 判断**（一次轻量调用）：让模型二分类"是否需要查阅历史记忆"，语义准但多一次调用——划算与否看 embedding 和 LLM 谁贵
3. **混合**（企业常见）：规则先过滤明显的，拿不准的交模型判

注意闸门的价值主要是**召回质量**（少翻旧账少噪音），其次才是省 embedding 成本。本项目落点：ReAct Run 开头的经验自召回处加规则级闸门。
