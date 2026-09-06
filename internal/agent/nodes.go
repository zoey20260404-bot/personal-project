package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"ai-start/configs"
	"ai-start/internal/llm"
	"ai-start/internal/prompt"
	"ai-start/internal/runtime"
	"ai-start/internal/tool"
	"ai-start/internal/types"
)

// parseTask Parser 节点的任务消息体（Content 为 JSON）。
// 支持多源融合：文本 + 多张图片（毕业证/学位证）一次解析。
type parseTask struct {
	Content string             `json:"content"` // 文本内容（可选）
	Images  []types.ImageInput `json:"images"`  // 图片列表（可选，当前仅支持 diploma_image）
}

// BuildRuntime 按配置装配多 Agent 运行时（去中心化总线架构）。
// 每个 Agent 配置变成一个独立 Node，各自 goroutine 运行，通过总线通信。
// 新增 Agent：在配置 agents 表中加定义（新类型时在 switch 中注册构造逻辑）。
func BuildRuntime(agentCfgs map[string]config.AgentConfig, runtimeCfg config.RuntimeConfig,
	manager *llm.Manager, prompts *prompt.Store, tools *tool.Registry, logger *slog.Logger) (*runtime.Runtime, error) {

	rt := runtime.NewRuntime(runtimeCfg.BusBuffer, logger)

	for name, ac := range agentCfgs {
		switch ac.Type {
		case AgentTypeParser:
			rt.Register(NewParserNode(name, ac, manager, prompts, runtimeCfg.NodeInbox))
		case AgentTypeReact:
			react := NewReActAgent(name, ac.Model, prompts, ac.Prompt, ac.MaxSteps, ac.Temperature, manager, tools, ac.Tools, logger)
			rt.Register(NewReActNode(name, react, runtimeCfg.NodeInbox))
		case AgentTypeRouter:
			router := NewRouterAgent(manager, ac.Model, prompts, "advisor") // 兜底目标：选岗参谋
			rt.Register(NewRouterNode(name, router, runtimeCfg.NodeInbox))
		default:
			return nil, fmt.Errorf("Agent %q 类型 %q 未支持", name, ac.Type)
		}
	}
	return rt, nil
}

// NewRouterNode 创建意图路由节点：任务为用户问题，回复为路由结果 JSON。
func NewRouterNode(name string, router *RouterAgent, inboxSize int) *runtime.Node {
	return runtime.NewNode(name, inboxSize, func(ctx context.Context, msg runtime.Message) (runtime.Message, error) {
		result := router.Route(ctx, msg.Content)
		data, err := json.Marshal(result)
		if err != nil {
			return runtime.Message{}, fmt.Errorf("序列化路由结果失败: %w", err)
		}
		return runtime.Message{Type: runtime.MsgTypeResult, Content: string(data)}, nil
	})
}

// NewParserNode 创建条件解析节点：接收多源解析任务（文本+图片），融合返回结构化 ParseResult。
func NewParserNode(name string, cfg config.AgentConfig, manager *llm.Manager, prompts *prompt.Store, inboxSize int) *runtime.Node {
	parser := NewParserAgent(manager, cfg.Model, prompts, cfg.Prompt)
	return runtime.NewNode(name, inboxSize, func(ctx context.Context, msg runtime.Message) (runtime.Message, error) {
		var task parseTask
		if err := json.Unmarshal([]byte(msg.Content), &task); err != nil {
			return runtime.Message{}, fmt.Errorf("解析任务消息失败: %w", err)
		}
		if task.Content == "" && len(task.Images) == 0 {
			return runtime.Message{}, fmt.Errorf("任务为空：文本与图片至少提供一项")
		}

		result, err := parser.ParseMulti(ctx, task.Content, task.Images)
		if err != nil {
			return runtime.Message{}, err
		}
		data, err := json.Marshal(result)
		if err != nil {
			return runtime.Message{}, fmt.Errorf("序列化解析结果失败: %w", err)
		}
		return runtime.Message{Type: runtime.MsgTypeResult, Content: string(data)}, nil
	})
}

// NewReActNode 创建 ReAct 节点：任务内容为自然语言输入，回复为最终回答。
func NewReActNode(name string, react *ReActAgent, inboxSize int) *runtime.Node {
	return runtime.NewNode(name, inboxSize, func(ctx context.Context, msg runtime.Message) (runtime.Message, error) {
		answer, err := react.Run(ctx, msg.Content)
		if err != nil {
			return runtime.Message{}, err
		}
		return runtime.Message{Type: runtime.MsgTypeResult, Content: answer}, nil
	})
}
