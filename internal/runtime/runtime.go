package runtime

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ErrNodePanic 节点处理消息时崩溃。
var ErrNodePanic = errors.New("节点处理消息时发生 panic")

// ErrNodeNotFound 目标节点不存在。
var ErrNodeNotFound = errors.New("目标 Agent 节点不存在")

// Runtime 多 Agent 运行时：持有消息总线与所有节点，
// 负责节点生命周期（启动/优雅退出）与消息分发。
type Runtime struct {
	bus    *Bus             // 共享消息总线
	nodes  map[string]*Node // 节点名 → 节点
	logger *slog.Logger     // 日志器（注入，注册节点时透传给节点）
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRuntime 创建运行时，busBuffer 为总线缓冲容量，logger 为共享日志器。
func NewRuntime(busBuffer int, logger *slog.Logger) *Runtime {
	return &Runtime{
		bus:    NewBus(busBuffer),
		nodes:  make(map[string]*Node),
		logger: logger,
	}
}

// Register 注册节点（启动前调用），并向节点注入运行时日志器。
func (r *Runtime) Register(node *Node) {
	node.logger = r.logger
	r.nodes[node.Name] = node
}

// Bus 暴露总线（供 Supervisor 等发起请求-响应调用）。
func (r *Runtime) Bus() *Bus {
	return r.bus
}

// Start 启动所有节点（各自独立 goroutine）与消息分发协程。
func (r *Runtime) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)

	// 消息分发协程：把总线消息路由到目标节点 Inbox（非阻塞，满则丢弃计入背压）
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-r.bus.ch:
				// 请求-响应配对的消息直接投递给调用方，不进入节点
				if r.bus.deliver(msg) {
					continue
				}
				r.dispatch(msg)
			}
		}
	}()

	// 启动全部节点
	for _, node := range r.nodes {
		r.wg.Add(1)
		go func(n *Node) {
			defer r.wg.Done()
			n.Run(ctx, r.bus)
		}(node)
	}
	r.logger.Info("Agent 节点已全部启动", "count", len(r.nodes))
}

// dispatch 按 To 字段路由消息：空为广播（投递给除发送方外的所有节点）。
func (r *Runtime) dispatch(msg Message) {
	if msg.To != "" {
		node, ok := r.nodes[msg.To]
		if !ok {
			r.logger.Warn("消息目标节点不存在", "to", msg.To, "trace_id", msg.TraceID)
			return
		}
		r.tryDeliver(node, msg)
		return
	}
	// 广播：投递给除发送方外的所有节点
	for _, node := range r.nodes {
		if node.Name == msg.From {
			continue
		}
		r.tryDeliver(node, msg)
	}
}

// tryDeliver 非阻塞投递到节点 Inbox；满则丢弃（背压：不阻塞分发协程）。
func (r *Runtime) tryDeliver(node *Node, msg Message) {
	select {
	case node.Inbox <- msg:
	default:
		r.logger.Warn("节点收件箱已满，消息丢弃", "node", node.Name, "trace_id", msg.TraceID)
		r.bus.dropped.Add(1)
	}
}

// Call 同步调用目标节点（请求-响应），timeout 为等待回复的超时时间。
func (r *Runtime) Call(ctx context.Context, from, to, msgType, content string, timeout time.Duration) (Message, error) {
	if _, ok := r.nodes[to]; !ok {
		return Message{}, ErrNodeNotFound
	}
	return r.bus.Call(ctx, from, to, msgType, content, timeout)
}

// Shutdown 优雅退出：取消所有节点 context 并等待退出完成。
func (r *Runtime) Shutdown() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
	r.logger.Info("所有 Agent 节点已退出")
}
