# feat004 技术设计：选岗推荐（冲稳保主链路）

> 对应需求文档：[advise-prd.md](advise-prd.md)
> 本文档随开发过程同步更新（见 docs/rule.md 第 3 条）。

## 1. 总体架构

```plain
chat: "帮我生成选岗报告"
  ↓ advisor（ReAct）调 run_advise 工具
AdviseService.Run
  ↓ FlowExecutor（flows 表配置：advise: [researcher, analyzer, strategist, responder]）
  researcher（代码节点：六维硬条件 SQL 检索，候选不足自动放宽）
  → analyzer（LLM：竞争分析 + 备注暗坑解读）
  → strategist（LLM 低温：冲稳保分组）
  → responder（LLM：按 mode 渲染报告，SSE 流式）
  ↓
reports 表落库 + SSE 进度/内容回显
```

## 2. 关键设计

### 2.1 新增节点类型

- `code`：纯代码节点（researcher），不调 LLM
- `llm_step`：单次 LLM 调用节点（analyzer/strategist/responder），非 ReAct 循环

### 2.2 工具边界

链路内部零工具调用（步骤固定，数据走消息体）；仅 `run_advise` 一个触发工具（advisor 白名单）。
参考 README「AI 能力边界划分原则」：调用时机固定的不暴露为工具。

### 2.3 异步与回显

run_advise 同步执行 flow（复用 SSE 连接流式回显进度与报告），报告落库。
RabbitMQ 等任务队列为后续可选增强（多实例/持久化时引入，见 README Bus vs MQ 问答）。

### 2.4 降级策略

| 失败点 | 降级 |
| --- | --- |
| 无档案 | 返回"先完成条件解析"指引 |
| Researcher 无候选 | 放宽专业/地域各重试一次，仍无 → 报告"建议放宽条件" |
| Analyzer/Strategist LLM 失败 | 规则化简版分组（按分数线排序凑档） |
| Responder 失败 | 返回结构化 JSON 报告（无润色） |

## 3. 数据设计

`reports` 表：report_id（唯一）、user_id、session_id、profile 快照、content（markdown）、result（结构化策略 JSON）、status（processing/completed/failed）、时间戳。

## 4. 接口

| 接口 | 方法 | 说明 |
| --- | --- | --- |
| `/api/v1/chat` | POST | 触发入口（run_advise 工具） |
| `/api/v1/reports` | GET | 报告列表（当前用户） |
| `/api/v1/reports/{id}` | GET | 报告详情 |

## 5. Prompt 设计（prompt 常量）

- `analyzer`：竞争分析（PRD 6.4）
- `strategist`：冲稳保分组（PRD 6.5）
- `responder`：报告生成（PRD 6.6，{{.Mode}} 变体）
