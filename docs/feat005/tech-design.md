# feat005 技术设计：结构化面试考官与聊天输入体验

> 分支：`feature/chenziyu/feat005` · 日期：2026-09-07
> 对应：[需求文档](interviewer-prd.md) · [问题记录](problem-solve.md)。

## 1. 现有框架接入

继续使用 `POST /api/v1/chat → ChatService → Router → ReAct → SSE`。不增加 API 字段、节点类型、状态表、工具或独立前端入口。

`internal/prompt/prompts_interviewer.go` 定义 `PromptInterviewer`，在 `names.go`、`defaults.go` 注册，复用最新配置已有的 `agents.interviewer`：`type: react`、`model: chat`、`prompt: interviewer`、`tools: []`、`max_steps: 1`、`temperature: 0.5`。通过原有 BuildRuntime 装配。

## 2. 跨轮路由与面试行为

`internal/api/logic/chat.go` 复用 composeInput，将最近历史与当前问题同时交给 Router 和目标节点，历史读取从 10 条扩大为缓冲已有的 20 条。Router Prompt 识别情境作答、继续、追问、结束等承接消息，尊重明确的新任务。

面试模式、题号及追问进度由 Prompt 根据历史恢复，每次回复标明模式与题号。练习模式即时点评，模拟模式结束后统一点评；`mode` 字段保留原 beginner/advanced 含义。没有独立面试状态持久化，超出历史窗口时不能保证恢复全部答案，必须说明缺失并要求补充。

## 3. 工具隔离

`internal/agent/react.go` 在工具执行前检查当前 Agent 的工具白名单。即使模型伪造全局已注册工具名，无工具面试节点也不会执行；失败沿用 ChatService 的兜底回复。

## 4. Web 实现

仅调整 `web/index.html` 的原聊天区域，保留现有 session_id、接口与 SSE 消费机制。

- textarea 支持多行，Enter 换行、Ctrl/⌘+Enter 发送，并检查 isComposing。
- `chatSending` 防止同一页面重复发送；finally 恢复按钮，请求异常且无新草稿时恢复原输入。
- 消息使用 pre-wrap 保留换行，overflow-wrap 防止长文本溢出。
- 输入容器最大宽度 1080px，居中、圆角 18px，focus-within 显示聚焦反馈；发送按钮固定高 34px，放在容器右下角。
- textarea 初始两行，输入时按 scrollHeight 自动增高，上限 180px，发送后恢复初始高度；底栏独立展示快捷键提示。600px 以下减少间距，聊天消息区允许收缩并使用细滚动条。

## 5. 配置与验证

已对比本地 config 与 example 的 87 个配置项，仅 chat、vision、embedding 的 api_key 不同；这些本地密钥不进入 example。今后配置变更按 `docs/rule.md` 同步非私密信息。

- `go test ./...` 已通过：面试相关测试覆盖最新示例配置装配、Prompt 渲染、上下文跨总线传递、允许切换路由目标及工具隔离。
- 前端脚本检查通过；模拟 DOM/fetch 检查同一 chat 接口、重复发送保护、SSE 回复和 session 保存、失败草稿恢复。
- 后续输入区样式调整通过脚本语法和 diff 检查，运行中首页已确认包含新输入容器。
- 本地服务已重启，`/health` 和 `/` 返回 200。自动化检查不代表真实模型语义或浏览器视觉验收完成。
