package agent

import "context"

// Agent 通用 Agent 接口：所有 Agent 的统一抽象。
// 注意：生产链路中 Agent 以 Node 形式接入消息总线（见 nodes.go），
// 本接口用于纯函数式 Agent 的直接调用场景。
type Agent interface {
	// Name Agent 标识。
	Name() string
	// Run 执行 Agent 核心逻辑，input 为用户输入或上游 Agent 输出。
	Run(ctx context.Context, input string) (string, error)
}

// Agent 类型常量：配置 agents 表中 type 字段的合法取值。
const (
	AgentTypeParser = "parser" // 条件解析 Agent
	AgentTypeReact  = "react"  // ReAct 推理-行动循环 Agent
)
