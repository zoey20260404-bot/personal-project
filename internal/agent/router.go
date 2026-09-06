package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"ai-start/internal/llm"
	"ai-start/internal/prompt"
)

// RouteResult Router 节点的路由结果。
type RouteResult struct {
	Target     string  `json:"target"`     // 目标 Agent 节点名
	Confidence float64 `json:"confidence"` // 意图置信度 0-1
	Reason     string  `json:"reason"`     // 判断理由（审计用）
	Clarify    bool    `json:"clarify"`    // true 表示意图不明确，应向用户反问
}

// routeConfidenceThreshold 路由置信度阈值，低于则要求澄清。
const routeConfidenceThreshold = 0.7

// RouterAgent 意图路由 Agent：LLM 判断用户消息应由哪个 Agent 处理。
type RouterAgent struct {
	llm      *llm.Manager
	channel  string
	prompts  *prompt.Store
	fallback string // LLM 不可用/失败时的兜底目标
}

// NewRouterAgent 创建意图路由 Agent。fallback 为兜底目标节点（通常 advisor）。
func NewRouterAgent(manager *llm.Manager, channel string, prompts *prompt.Store, fallback string) *RouterAgent {
	if channel == "" {
		channel = llm.ChannelChat
	}
	return &RouterAgent{llm: manager, channel: channel, prompts: prompts, fallback: fallback}
}

// Route 判断意图并返回路由结果。
// 容错兜底：LLM 不可用/输出非法/低置信度 → Clarify 或兜底目标，绝不报错中断。
func (r *RouterAgent) Route(ctx context.Context, question string) RouteResult {
	// LLM 不可用：直接兜底目标（通常是选岗参谋）
	if !r.llm.Has(r.channel) {
		return RouteResult{Target: r.fallback, Confidence: 1.0, Reason: "LLM 未配置，走默认路由"}
	}

	out, err := r.llm.ChatCompletion(ctx, r.channel,
		[]llm.Message{{Role: "user", Content: question}},
		&llm.Options{System: r.prompts.Get(prompt.NameRouter)})
	if err != nil {
		return RouteResult{Target: r.fallback, Confidence: 1.0, Reason: "路由模型调用失败，走默认路由"}
	}
	return r.parseRoute(out.Content)
}

// parseRoute 解析路由输出（容错：非 JSON/缺字段时兜底）。
func (r *RouterAgent) parseRoute(out string) RouteResult {

	var parsed struct {
		Agent      string      `json:"agent"`
		Confidence interface{} `json:"confidence"` // 容错：可能是数字或字符串
		Reason     string      `json:"reason"`
	}
	if err := json.Unmarshal([]byte(extractJSON(out)), &parsed); err != nil || parsed.Agent == "" {
		return RouteResult{Target: r.fallback, Confidence: 1.0, Reason: "路由输出解析失败，走默认路由"}
	}

	conf := toFloat(parsed.Confidence)
	if conf < routeConfidenceThreshold {
		return RouteResult{Clarify: true, Confidence: conf, Reason: parsed.Reason}
	}
	return RouteResult{Target: parsed.Agent, Confidence: conf, Reason: parsed.Reason}
}

// toFloat 容错转换 interface{} 为 float64（数字或数字字符串）。
func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		var f float64
		fmt.Sscanf(n, "%f", &f)
		return f
	}
	return 0
}
