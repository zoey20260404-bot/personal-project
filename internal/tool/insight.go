package tool

import (
	"context"
	"encoding/json"
	"errors"

	"ai-start/internal/runtime"
)

// SaveInsightFunc 经验沉淀能力（由 agent 记忆模块注入实现）。
type SaveInsightFunc func(ctx context.Context, agentName, content string)

// SaveInsightTool 经验沉淀工具：Agent 自主决定将"值得长期记住的经验"写入自己的记忆空间。
// Agent 名从执行上下文取（节点注入），模型无法冒充其他 Agent 写记忆。
type SaveInsightTool struct {
	save SaveInsightFunc
}

// NewSaveInsightTool 创建经验沉淀工具。
func NewSaveInsightTool(save SaveInsightFunc) *SaveInsightTool {
	return &SaveInsightTool{save: save}
}

// Name 工具名。
func (t *SaveInsightTool) Name() string { return "save_insight" }

// Description 功能描述。
func (t *SaveInsightTool) Description() string {
	return "沉淀一条值得长期记住的经验或发现（如用户偏好规律、岗位竞争规律）。仅在有真正有价值的发现时调用，闲聊不要调用"
}

// ParamsJSON 参数 JSON Schema。
func (t *SaveInsightTool) ParamsJSON() string {
	return `{
		"type": "object",
		"properties": {
			"content": {"type": "string", "description": "经验内容，一句话描述清楚，如 用户连续3次关注深圳岗位，偏好珠三角"}
		},
		"required": ["content"]
	}`
}

// Execute 执行沉淀（写入当前 Agent 自己的 agent 作用域记忆）。
func (t *SaveInsightTool) Execute(ctx context.Context, args string) (string, error) {
	agentName := runtime.AgentNameFromContext(ctx)
	if agentName == "" {
		return "", errors.New("缺少 Agent 上下文")
	}
	var params struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil || params.Content == "" {
		return "", errors.New("参数错误：content 必填")
	}
	t.save(ctx, agentName, params.Content)
	return `{"message": "经验已沉淀"}`, nil
}
