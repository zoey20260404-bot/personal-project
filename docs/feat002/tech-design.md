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
| P1 | Router + advisor + 四个工具 + chat 接口 | 待 TODO-2 确认范围后启动 |
| P2 | 记忆接入 + SSE 流式 | 待启动 |
| P3 | interviewer（纯 Prompt，最简单） | 待启动 |
| P4 | exam_coach（依赖 TODO-1 题库数据源） | 阻塞 |
| P5 | group_discussion（编排最复杂） | 待细化（TODO-3） |
