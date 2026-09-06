// Package agent 多 Agent 运行时（v1 手写实现，不依赖外部 Agent 框架）。
//
// 架构：去中心化协作（Decentralized）——所有 Agent 是对等节点，
// 各自在独立 goroutine 中运行，通过共享消息总线（Bus）通信；
// 层级调度（Supervisor 分发任务）与流水线（消息链）是总线上的使用模式而非硬编码结构。
package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// 消息类型。
const (
	MsgTypeTask   = "task"   // 任务派发
	MsgTypeResult = "result" // 处理结果
	MsgTypeError  = "error"  // 处理失败
	MsgTypeReview = "review" // 评审意见（Reviewer 类 Agent 使用）
)

// ErrBusFull 总线缓冲已满（背压触发）。
var ErrBusFull = errors.New("消息总线缓冲已满")

// ErrReplyTimeout 等待节点回复超时。
var ErrReplyTimeout = errors.New("等待 Agent 回复超时")

// Message Agent 间通信协议。
type Message struct {
	ID        string    // 关联 ID：响应与请求同 ID，用于请求-响应配对
	TraceID   string    // 链路追踪 ID（一次外部请求贯穿所有 Agent 消息）
	UserID    uint64    // 用户身份（JWT 透传，跨总线传播；工具执行的身份来源）
	From      string    // 发送方节点名
	To        string    // 接收方节点名，空表示广播
	Type      string    // task / result / error / review
	Content   string    // 消息体（业务数据，通常为 JSON）
	CreatedAt time.Time // 创建时间
}

// newMsgID 生成消息关联 ID。
func newMsgID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// pendingEntry 请求-响应配对条目。
type pendingEntry struct {
	replyCh chan Message // 调用方等待回复的通道
	target  string       // 目标节点名：只有来自该节点的消息才算回复（防止请求自身被误配对）
	sink    EventSink    // 流式事件回调（跨总线透传：节点事件经总线转发给调用方）
}

// Bus 共享消息总线：缓冲 channel + 非阻塞发送（背压控制）。
// 节点发布消息到总线，由分发协程按 To 字段路由到目标节点 Inbox。
type Bus struct {
	ch      chan Message  // 总线缓冲通道
	pending sync.Map      // 请求-响应配对：消息 ID → pendingEntry
	dropped atomic.Uint64 // 因缓冲满被丢弃的消息数（背压监控指标）
}

// NewBus 创建消息总线，bufferSize 为缓冲容量。
func NewBus(bufferSize int) *Bus {
	return &Bus{ch: make(chan Message, bufferSize)}
}

// Publish 非阻塞发布消息；缓冲满时丢弃并计入 dropped（背压：宁可丢弃也不阻塞 Agent）。
func (b *Bus) Publish(msg Message) {
	if msg.ID == "" {
		msg.ID = newMsgID()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	select {
	case b.ch <- msg:
	default:
		b.dropped.Add(1)
	}
}

// Dropped 返回因背压被丢弃的消息总数。
func (b *Bus) Dropped() uint64 {
	return b.dropped.Load()
}

// Call 同步请求-响应：向目标节点发送任务并等待同 ID 的回复。
// 供 API 层/Supervisor 使用，把异步总线包装成同步调用。
// sink 非空时，节点处理过程中产生的流式事件会实时转发给调用方。
func (b *Bus) Call(ctx context.Context, from, to, msgType, content string, timeout time.Duration, sink EventSink) (Message, error) {
	msg := Message{
		ID:      newMsgID(),
		From:    from,
		To:      to,
		Type:    msgType,
		Content: content,
	}
	// 从上下文继承链路追踪 ID 与用户身份（跨总线传播）
	if traceID, ok := ctx.Value(traceIDKey{}).(string); ok {
		msg.TraceID = traceID
	}
	msg.UserID = UserIDFromContext(ctx)

	replyCh := make(chan Message, 1)
	b.pending.Store(msg.ID, pendingEntry{replyCh: replyCh, target: to, sink: sink})
	defer b.pending.Delete(msg.ID)

	b.Publish(msg)

	select {
	case reply := <-replyCh:
		if reply.Type == MsgTypeError {
			return reply, errors.New(reply.Content)
		}
		return reply, nil
	case <-time.After(timeout):
		return Message{}, fmt.Errorf("%w: %s", ErrReplyTimeout, to)
	case <-ctx.Done():
		return Message{}, ctx.Err()
	}
}

// EmitToMessage 将流式事件转发给指定消息的调用方（节点事件跨总线回传 SSE）。
func (b *Bus) EmitToMessage(msgID string, ev StreamEvent) {
	if entry, ok := b.pending.Load(msgID); ok {
		if sink := entry.(pendingEntry).sink; sink != nil {
			sink(ev)
		}
	}
}

// deliver 回复路由：命中 pending 且消息确实来自目标节点时，投递到调用方回复通道。
// 返回 true 表示已被配对消费，不再路由给节点。
func (b *Bus) deliver(msg Message) bool {
	// 只有结果类消息才可能是回复；task 类型一律不是（防止请求自身/节点自调用被误配对）
	if msg.Type == MsgTypeTask {
		return false
	}
	entry, ok := b.pending.Load(msg.ID)
	if !ok {
		return false
	}
	pe := entry.(pendingEntry)
	if msg.From != pe.target {
		return false // 来源不符的消息不算回复，继续正常路由
	}
	select {
	case pe.replyCh <- msg:
	default: // 回复通道无人接收（调用方已超时），丢弃
	}
	return true
}
