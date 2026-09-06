package runtime

import "context"

// StreamEvent 流式事件（SSE 用）：跨层级经 context 传递 sink，实现
// HTTP handler ← ChatService ← 总线 ← 节点 ← ReAct 的逐级事件透传。
type StreamEvent struct {
	Type    string `json:"type"`            // status / delta / tool / done / error
	Content string `json:"content"`         // 事件内容
	Agent   string `json:"agent,omitempty"` // 事件来源 Agent（可选）
	Tool    string `json:"tool,omitempty"`  // 工具名（type=tool 时）
}

// 事件类型常量。
const (
	EventStatus = "status" // 状态提示（如"意图识别中"）
	EventDelta  = "delta"  // 回答文本增量（逐 token）
	EventTool   = "tool"   // 工具调用提示
	EventDone   = "done"   // 完成
	EventError  = "error"  // 错误
)

// eventSinkKey 流式事件回调的 context 键。
type eventSinkKey struct{}

// EventSink 流式事件回调。
type EventSink func(StreamEvent)

// WithEventSink 将事件回调注入 context（HTTP 层设置，ReAct/节点内触发）。
func WithEventSink(ctx context.Context, sink EventSink) context.Context {
	return context.WithValue(ctx, eventSinkKey{}, sink)
}

// EmitEvent 向 sink 发送事件（无 sink 时空操作）。
func EmitEvent(ctx context.Context, eventType, content string) {
	if sink, ok := ctx.Value(eventSinkKey{}).(EventSink); ok && sink != nil {
		sink(StreamEvent{Type: eventType, Content: content})
	}
}

// EmitToolEvent 发送工具调用事件。
func EmitToolEvent(ctx context.Context, agentName, toolName string) {
	if sink, ok := ctx.Value(eventSinkKey{}).(EventSink); ok && sink != nil {
		sink(StreamEvent{Type: EventTool, Agent: agentName, Tool: toolName})
	}
}

// SinkFromContext 取事件回调（ReAct 判断是否需要流式输出用）。
func SinkFromContext(ctx context.Context) EventSink {
	if sink, ok := ctx.Value(eventSinkKey{}).(EventSink); ok {
		return sink
	}
	return nil
}
