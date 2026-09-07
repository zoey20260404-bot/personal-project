package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"ai-start/internal/llm"
	"ai-start/internal/prompt"
	"ai-start/internal/runtime"
	"ai-start/internal/tool"
)

// LLMCaller ReAct Agent 依赖的模型调用接口（便于单测 mock）。
type LLMCaller interface {
	// ChatCompletion 指定渠道发起对话，支持工具定义与参数覆盖。
	ChatCompletion(ctx context.Context, channel string, messages []llm.Message, opts *llm.Options) (*llm.Message, error)
}

// StreamCaller 支持流式输出的模型调用接口（llm.Manager 实现）。
type StreamCaller interface {
	ChatStream(ctx context.Context, channel string, messages []llm.Message, opts *llm.Options, onDelta func(string)) (*llm.Message, error)
}

// ReActAgent 手写 ReAct（推理-行动循环）Agent：
// 模型推理 → 发起工具调用 → 执行工具 → 把结果喂回模型 → 再推理，直至给出最终回答或达到步数上限。
// Prompt 不固定：每次运行从 Prompt 库现取，可按场景切换变体。
type ReActAgent struct {
	name        string         // Agent 名
	channel     string         // 模型渠道名
	prompts     *prompt.Store  // Prompt 模板库
	promptName  string         // Prompt 名（可按场景换用其他变体）
	maxSteps    int            // 最大推理步数（防止死循环）
	temperature *float64       // 温度覆盖（可选，nil 用渠道配置）
	llm         LLMCaller      // 模型调用入口
	tools       *tool.Registry // 工具注册表
	toolDefs    []llm.ToolDef  // 本 Agent 可用的工具定义
	logger      *slog.Logger   // 日志器（注入）
	memory      *Memory        // 长期记忆（Agent 自管经验，nil 停用）
}

// NewReActAgent 创建 ReAct Agent。logger 为空时使用 slog.Default()。
// memory 为该 Agent 的长期记忆（经验自召回/自沉淀），nil 停用。
func NewReActAgent(name, channel string, prompts *prompt.Store, promptName string, maxSteps int, temperature *float64, caller LLMCaller, tools *tool.Registry, toolNames []string, logger *slog.Logger, memory *Memory) *ReActAgent {
	if maxSteps <= 0 {
		maxSteps = 5 // 默认最大步数
	}
	if logger == nil {
		logger = slog.Default()
	}
	a := &ReActAgent{
		name:        name,
		channel:     channel,
		prompts:     prompts,
		promptName:  promptName,
		maxSteps:    maxSteps,
		temperature: temperature,
		llm:         caller,
		tools:       tools,
		logger:      logger,
		memory:      memory,
	}
	// 按配置的工具名导出工具定义
	for _, d := range tools.Defs(toolNames) {
		a.toolDefs = append(a.toolDefs, llm.ToolDef{
			Name:        d.Name,
			Description: d.Description,
			ParamsJSON:  d.ParamsJSON,
		})
	}
	return a
}

// Name Agent 标识。
func (a *ReActAgent) Name() string { return a.name }

// Run 执行 ReAct 循环，返回最终文本回答。
func (a *ReActAgent) Run(ctx context.Context, input string) (string, error) {
	// 经验自召回（feat003）：Agent 开头召回自己的长期记忆（agent/user 作用域），
	// 与编排层的短期历史互补——编排层管"刚才聊了啥"，Agent 管"我记得什么"。
	vars := runtime.PromptVarsFromContext(ctx)
	if a.memory != nil {
		if userID := runtime.UserIDFromContext(ctx); userID != 0 {
			if recalled := a.memory.Recall(ctx, a.name, userID, "", input, 5); len(recalled) > 0 {
				if vars == nil {
					vars = map[string]any{}
				}
				var lines []string
				for _, r := range recalled {
					lines = append(lines, "- "+r.Content)
				}
				vars["Memories"] = strings.Join(lines, "\n")
			}
		}
	}

	// 每次运行现取并渲染系统提示词（动态 Prompt：变量来自 context，可随时切换变体）
	system := ""
	if a.prompts != nil {
		if rendered, err := a.prompts.Render(a.promptName, vars); err == nil {
			system = rendered
		} else {
			system = a.prompts.Get(a.promptName) // 渲染失败回退原文
		}
	}

	messages := []llm.Message{{Role: "user", Content: input}}
	opts := &llm.Options{System: system, Tools: a.toolDefs, Temperature: a.temperature}

	// 有事件 sink 且渠道支持流式时逐 token 输出；否则普通调用
	streamer, canStream := a.llm.(StreamCaller)
	streaming := canStream && runtime.SinkFromContext(ctx) != nil
	onDelta := func(delta string) { runtime.EmitEvent(ctx, runtime.EventDelta, delta) }

	for step := 1; step <= a.maxSteps; step++ {
		var resp *llm.Message
		var err error
		if streaming {
			resp, err = streamer.ChatStream(ctx, a.channel, messages, opts, onDelta)
		} else {
			resp, err = a.llm.ChatCompletion(ctx, a.channel, messages, opts)
		}
		if err != nil {
			return "", fmt.Errorf("ReAct 第 %d 步模型调用失败: %w", step, err)
		}

		// 无工具调用：推理结束，返回最终回答
		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// 记录 assistant 的工具调用消息，随后逐个执行工具并回填结果
		messages = append(messages, *resp)
		for _, call := range resp.ToolCalls {
			// 无工具面试节点也必须在执行层隔离，不能信任模型返回的工具名。
			allowed := false
			for _, def := range a.toolDefs {
				if def.Name == call.Name {
					allowed = true
					break
				}
			}
			if !allowed {
				return "", fmt.Errorf("Agent %s 无权调用工具 %q", a.name, call.Name)
			}
			runtime.EmitToolEvent(ctx, a.name, call.Name) // 前端可见"正在调用工具"
			a.logger.Info("ReAct 调用工具",
				"agent", a.name, "step", step, "tool", call.Name, "args", call.Arguments, "trace_id", runtime.TraceIDFromContext(ctx))
			result := a.tools.Execute(ctx, call.Name, call.Arguments)
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
		}
	}
	// 达到步数上限：返回提示，调用方可视为部分完成
	return "", fmt.Errorf("ReAct 达到最大步数 %d 仍未得出最终回答", a.maxSteps)
}
