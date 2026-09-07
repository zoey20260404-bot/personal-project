# feat002 技术设计：多轮追问（多 Agent 对话中枢）

> 对应需求文档：[chat-agent-prd.md](chat-agent-prd.md)
> 本文档随开发过程同步更新（见 docs/rule.md 第 3 条）。

## 1. 总体架构

复用 feat001 的去中心化总线运行时，chat 链路新增 Router 节点与各业务 Agent 节点：

```plain
POST /api/v1/chat（JWT）
  ↓ ChatService（logic 层）
  ↓ Runtime.Call(to="router")
Router 节点（LLM 意图识别 → 输出目标 Agent 名 + 置信度）
  ↓ 按结果 Runtime.Call(to=目标Agent)
┌────────────┬────────────┬────────────┐
│ advisor     │ exam_coach │ interviewer│  …（配置化注册）
│ （选岗参谋）  │ （笔试中枢） │ （面试考官） │
└────────────┴────────────┴────────────┘
  各 Agent 为 ReAct 循环（现有 ReActAgent），带各自工具白名单
```

## 2. 关键设计

### 2.1 意图路由

- Router 是一个轻量 LLM 节点：系统 Prompt 列出各 Agent 职责，输出 JSON `{agent, confidence, reason}`
- confidence < 阈值（默认 0.7）→ 不路由，直接返回反问澄清文案
- 路由结果带 TraceID 记日志，可审计"为什么走了这个 Agent"

### 2.2 工具权限（配置级白名单）

复用现有 `agents.*.tools` 配置项，ReActAgent 构造时只挂载白名单内工具（已实现）：

```yaml
agents:
  router:      { type: router, model: chat }
  advisor:     { type: react, model: chat, prompt: advisor, tools: [query_positions, get_profile, update_profile, add_favorite] }
  exam_coach:  { type: react, model: chat, prompt: exam_coach, tools: [query_questions, submit_answer] }
  interviewer: { type: react, model: chat, prompt: interviewer, tools: [] }
```

执行层安全：工具从 context 取 user_id（JWT 透传），模型参数不参与身份判定。

### 2.3 档案补充（update_profile 工具）

复用 feat001 既有能力，不新造逻辑：

```plain
update_profile(用户口述文本)
  → Parser Agent 解析文本（复用）
  → mergeWithExisting（新值覆盖明确字段，旧值保留未提及字段）
  → upsertProfile 落 user_profiles 表
  → 返回变更摘要（改了哪些字段）
```

### 2.4 记忆接入

- 每轮 chat：用户消息与 AI 回复写入记忆（scope=session）
- 生成前召回：历史对话（session）+ 画像（user）+ 经验（agent）合并进上下文
- 依赖 embedding 渠道配置（未配置时降级为无记忆对话）

### 2.5 流式输出

SSE（text/event-stream）：ReAct 循环中工具调用过程发"思考中"事件，最终回答流式输出。模型渠道需支持 stream（go-openai 支持）。

### 2.6 无领导小组（后续迭代）

特殊编排：非 ReAct，回合制——多个 persona Agent 依次发言（可见历史发言）→ 用户发言 → 考官点评。为独立流程类型，不占用 ReAct 框架。

## 3. 接口设计

| 接口 | 方法 | 说明 |
| --- | --- | --- |
| `/api/v1/chat` | POST | 对话入口。请求：`{session_id?, question, mode?}`；响应：SSE 流（或首轮同步返回，流式后补） |

## 4. 新增工具（tool 包）

| 工具 | 说明 | 包装 |
| --- | --- | --- |
| query_positions | 岗位查询 | PositionService.Query |
| get_profile | 取用户档案 | ParseService.GetLatestProfile |
| update_profile | 档案补充 | Parser 解析 + 档案合并 |
| add_favorite | 收藏岗位 | FavoriteService.Add |
| query_questions | 题库查询（TODO-1 依赖） | 待定 |

## 5. 与 feat001 的关系

- 复用：runtime/总线、ReActAgent、工具注册表、记忆层、Prompt 多变体、档案合并规则、JWT/TraceID
- 新增：router 节点类型、chat 接口、update_profile 等工具、SSE
- parse 页表单与 chat 补充是同一档案能力的两个入口

## 6. 实施阶段

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| P1 | Router + advisor + 四个工具 + chat 接口 | ✅ 已实现（2026-09-06） |
| P2 | 记忆接入（短期 Redis 缓冲 + 长期 pgvector 召回） | ✅ 已实现（记忆写入待 embedding 密钥配置后生效） |
| P2.5 | SSE 流式输出（status/tool/delta/done 事件，逐 token） | ✅ 已实现（2026-09-06，事件经总线 pending 条目跨节点透传） |
| P3 | interviewer（复用 ReAct，现有多轮追问入口） | 已编码，真实模型交互待验收 |
| P4 | exam_coach（依赖 TODO-1 题库数据源） | 阻塞 |
| P5 | group_discussion（编排最复杂） | 待细化（TODO-3） |

### P3 实现（2026-09-07）

- 沿用最新配置已有 `interviewer` 定义，补齐 `PromptInterviewer` 常量与注册表；两份配置的非敏感配置项已核对一致，无需覆盖本地密钥或节点。
- 仍走 ChatService → Router → ReAct → SSE；仅将 Router 输入改为与目标节点相同的历史+本轮问题，读取已有 20 条缓冲窗口。
- 练习/模拟模式由自然语言与历史恢复，`mode` 字段仍为 beginner/advanced 表达风格，不新增 API 或状态表。
- ReAct 执行前校验已有工具白名单，保证面试节点不执行模型伪造的工具调用。
- Web 仅修改现有聊天框的提示、textarea、换行展示和发送锁，不增加 Tab、按钮入口或独立会话流程。
- 离线测试覆盖配置装配、模板渲染、上下文传递及工具隔离；不等同于模型语义准确率验收。

### P1 落地说明（技术清单对照）

- **意图路由**：`agent.RouterAgent`（router 节点），LLM 输出 `{agent, confidence, reason}`，<0.7 反问澄清；模型不可用/输出非法 → 兜底 advisor
- **工具白名单**：`agents.*.tools` 配置生效，ReActAgent 只挂载白名单工具；用户身份经 `tool.WithUserID(ctx)` 从 JWT 透传，模型无法伪造
- **动态 Prompt**：`WithPromptVars(ctx, {Mode, Profile, Memories})`，ReAct 每次运行现渲染
- **记忆**：短期 Redis `chat:buf:{session}`（20 条/2h）；长期 pgvector 三层作用域双写双召回
- **ReAct 兜底**：max_steps 封顶 + Agent 失败返回友好兜底文案 + TraceID 日志
- **温度/参数控制**：router temperature=0.1（分类求稳）、advisor temperature=0.5（表达适度），渠道级可配 max_tokens
- **容错**：路由失败/节点未注册/ReAct 失败/LLM 无 key，全链路均有降级，接口不 500
- **去中心化**：router/advisor 均为总线对等节点，各自独立 goroutine（运行时启动 4 节点）
