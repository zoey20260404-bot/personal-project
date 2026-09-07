# feat003 技术设计：多 Agent 记忆体系

> 对应需求文档：[memory-prd.md](memory-prd.md)
> 本文档随开发过程同步更新（见 docs/rule.md 第 3 条）。

## 1. 改动总览

| 模块 | 改动 |
| --- | --- |
| `internal/store/vector.go` | `SearchMemories` 作用域语义修正：agent 按名隔离，session/user 不按 agent 过滤 |
| `internal/agent/react.go` | ReActAgent 增加 memory 依赖，Run 开头自召回经验注入 Prompt 变量 |
| `internal/agent/nodes.go` | BuildRuntime 传入 memory，注入各 ReAct 节点 |
| `internal/runtime/node.go` | 导出 `AgentNameFromContext`（节点名注入 ctx） |
| `internal/tool/insight.go` | 新增 `save_insight` 工具（Agent 自主沉淀，名字取自 ctx） |
| `internal/api/logic/chat.go` | 编排层瘦身：删除路由前向量召回 + 原始消息长期双写 |
| `internal/api/logic/parse.go` | 档案变更 → user 作用域记忆（规则触发） |
| `internal/api/logic/favorite.go` | 收藏行为 → user 作用域记忆（规则触发） |
| `internal/svc` | Memory 提前装配（ReAct 节点依赖）；注册 save_insight |
| `configs/config.yaml` | advisor 工具白名单加 `save_insight` |

## 2. 记忆读写链路

```plain
写入：
  规则触发（档案变更/收藏）──→ user 作用域（跨 Agent 共享）
  Agent 自主（save_insight）──→ agent 作用域（按名隔离）
  对话原文 ──→ Redis 短期缓冲（20 条 / 2h）

召回：
  ChatService：短期历史 + 画像摘要 → Prompt 变量
  ReActAgent.Run 开头：memory.Recall(自己的名字, userID, query)
    → agent 作用域（自己的经验）+ user 作用域（用户画像）
```

## 3. 关键 SQL 语义（store.SearchMemories）

```sql
WHERE (scope = 'agent'   AND agent_name = $1)   -- 经验：隔离
   OR (scope = 'user'    AND user_id = $2)      -- 画像：共享
   OR (scope = 'session' AND session_id = $3)   -- 会话：共享
```

## 4. 验证记录（2026-09-07）

- chat "我其实是团员" → advisor 调 update_profile → 档案更新 + user 作用域写入"政治面貌: 中共党员 → 共青团员" ✓
- 旧版 session 作用域原始消息已清理（16 条）
- 单测全绿（23+）
