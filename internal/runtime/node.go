package runtime

import (
	"context"
	"log/slog"
	"runtime/debug"
)

// traceIDKey 链路追踪 ID 的 context 键。
type traceIDKey struct{}

// agentNameKey Agent 名的 context 键（上下文隔离：标记当前执行归属哪个 Agent）。
type agentNameKey struct{}

// WithTraceID 将链路追踪 ID 注入 context。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// TraceIDFromContext 从 context 取链路追踪 ID，无则返回空串。
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
}

// Handler 节点消息处理函数：输入任务消息，输出回复消息。
type Handler func(ctx context.Context, msg Message) (Message, error)

// Node 对等 Agent 节点：独立 goroutine 运行，拥有自己的 Inbox，
// 通过共享总线与其他节点通信。每个节点有独立的上下文与记忆命名空间（上下文隔离）。
type Node struct {
	Name   string       // 节点名（唯一）
	Inbox  chan Message // 节点收件箱（带缓冲，背压控制）
	Handle Handler      // 消息处理逻辑
	// OnPanic 节点崩溃回调（可选），默认记日志后继续运行
	OnPanic func(name string, r interface{})

	logger *slog.Logger // 日志器（由 Runtime.Register 注入）
}

// NewNode 创建节点，inboxSize 为收件箱缓冲容量。
func NewNode(name string, inboxSize int, h Handler) *Node {
	return &Node{Name: name, Inbox: make(chan Message, inboxSize), Handle: h}
}

// Run 节点主循环：消费 Inbox 消息并处理，崩溃自动恢复（recover），
// context 取消时退出（优雅退出）。
func (n *Node) Run(ctx context.Context, bus *Bus) {
	// 上下文隔离：注入本节点名，节点内的日志/记忆操作互不污染
	ctx = context.WithValue(ctx, agentNameKey{}, n.Name)
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-n.Inbox:
			// 只处理发给自己的消息（广播由分发层展开，这里不再判断）
			reply, err := n.safeHandle(ctx, msg)
			if err != nil {
				reply = Message{From: n.Name, To: msg.From, Type: MsgTypeError, Content: err.Error()}
			}
			reply.ID = msg.ID // 关联 ID 透传，请求-响应配对
			reply.TraceID = msg.TraceID
			if reply.From == "" {
				reply.From = n.Name
			}
			if reply.To == "" {
				reply.To = msg.From
			}
			bus.Publish(reply)
		}
	}
}

// safeHandle 带崩溃保护的消息处理：单个消息处理 panic 不会拖垮节点。
func (n *Node) safeHandle(ctx context.Context, msg Message) (reply Message, err error) {
	defer func() {
		if r := recover(); r != nil {
			n.logger.Error("节点处理消息崩溃（已恢复）",
				"node", n.Name, "trace_id", msg.TraceID, "panic", r, "stack", string(debug.Stack()))
			if n.OnPanic != nil {
				n.OnPanic(n.Name, r)
			}
			err = ErrNodePanic
		}
	}()
	return n.Handle(ctx, msg)
}
