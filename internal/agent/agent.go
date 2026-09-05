// Package agent 是多 Agent 协作的核心编排层。
// 包含：条件解析、竞争分析、策略生成、多轮对话等 Agent 的定义与调度。
package agent

// Agent 定义了单个智能体的通用行为接口。
type Agent interface {
	// Name 返回 Agent 名称，用于日志与调度标识。
	Name() string
	// Run 执行 Agent 的核心逻辑，input 为用户输入或上游 Agent 的输出。
	Run(ctx Context, input string) (string, error)
}

// Context 承载一次会话的上下文信息（用户条件、历史消息等）。
// TODO: 按 PRD 补充字段：学历、专业、政治面貌、应届身份等。
type Context struct {
	SessionID string // 会话唯一标识
	UserID    string // 用户唯一标识
}
