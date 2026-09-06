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
    └── feat002/       # 多轮追问多 Agent 对话中枢（PRD + 技术文档，待启动）
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
