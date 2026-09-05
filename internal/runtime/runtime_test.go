package runtime

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// newTestRuntime 创建测试运行时：先注册节点再启动，并注册清理。
func newTestRuntime(t *testing.T, nodes ...*Node) *Runtime {
	t.Helper()
	rt := NewRuntime(64, slog.Default())
	for _, n := range nodes {
		rt.Register(n)
	}
	rt.Start(context.Background())
	t.Cleanup(rt.Shutdown)
	return rt
}

// TestRuntimeCallReply 验证请求-响应：Call 能收到目标节点同关联 ID 的回复。
func TestRuntimeCallReply(t *testing.T) {
	rt := newTestRuntime(t, NewNode("echo", 8, func(_ context.Context, msg Message) (Message, error) {
		return Message{Type: MsgTypeResult, Content: "echo: " + msg.Content}, nil
	}))

	reply, err := rt.Call(context.Background(), "test", "echo", MsgTypeTask, "hello", 3*time.Second)
	if err != nil {
		t.Fatalf("Call 失败: %v", err)
	}
	if reply.Content != "echo: hello" {
		t.Errorf("回复错误: got %q", reply.Content)
	}
	if reply.Type != MsgTypeResult {
		t.Errorf("消息类型错误: got %q", reply.Type)
	}
}

// TestCallNodeNotFound 验证调用不存在节点时报错。
func TestCallNodeNotFound(t *testing.T) {
	rt := NewRuntime(8, slog.Default())
	if _, err := rt.Call(context.Background(), "test", "ghost", MsgTypeTask, "x", time.Second); err != ErrNodeNotFound {
		t.Errorf("应返回 ErrNodeNotFound, got %v", err)
	}
}

// TestNodePanicRecovery 验证节点 panic 后自动恢复并回复 error 消息，且节点仍存活。
func TestNodePanicRecovery(t *testing.T) {
	rt := NewRuntime(64, slog.Default())
	calls := 0
	rt.Register(NewNode("flaky", 8, func(_ context.Context, msg Message) (Message, error) {
		calls++
		if calls == 1 {
			panic("模拟崩溃")
		}
		return Message{Type: MsgTypeResult, Content: "ok"}, nil
	}))
	rt.Start(context.Background())
	defer rt.Shutdown()

	// 第一次触发 panic，应返回 error（节点回发了 error 类型消息）
	if _, err := rt.Call(context.Background(), "test", "flaky", MsgTypeTask, "boom", 3*time.Second); err == nil {
		t.Error("panic 应返回 error")
	} else if !strings.Contains(err.Error(), "panic") {
		t.Errorf("错误内容应包含 panic 信息: %v", err)
	}

	// 第二次调用，节点应已从 panic 恢复并正常回复
	reply, err := rt.Call(context.Background(), "test", "flaky", MsgTypeTask, "again", 3*time.Second)
	if err != nil || reply.Content != "ok" {
		t.Errorf("节点未恢复: reply=%+v err=%v", reply, err)
	}
}

// TestBusBackpressure 验证背压：总线缓冲满时 Publish 不阻塞且计入丢弃数。
func TestBusBackpressure(t *testing.T) {
	bus := NewBus(1) // 缓冲容量 1，无消费者
	bus.Publish(Message{Content: "m1"})
	bus.Publish(Message{Content: "m2"}) // 缓冲已满，丢弃
	bus.Publish(Message{Content: "m3"}) // 丢弃
	if got := bus.Dropped(); got != 2 {
		t.Errorf("背压丢弃数错误: got %d, want 2", got)
	}
}

// TestSelfCall 验证节点自调用（from == to）：请求不应被自己冒领（deliver 的类型守卫）。
func TestSelfCall(t *testing.T) {
	rt := newTestRuntime(t, NewNode("echo", 8, func(_ context.Context, msg Message) (Message, error) {
		return Message{Type: MsgTypeResult, Content: "echo: " + msg.Content}, nil
	}))

	reply, err := rt.Call(context.Background(), "echo", "echo", MsgTypeTask, "self", 3*time.Second)
	if err != nil {
		t.Fatalf("自调用失败: %v", err)
	}
	if reply.Content != "echo: self" {
		t.Errorf("自调用回复错误: got %q", reply.Content)
	}
}

// TestCallTimeout 验证等待回复超时。
func TestCallTimeout(t *testing.T) {
	rt := NewRuntime(64, slog.Default())
	rt.Register(NewNode("slow", 8, func(ctx context.Context, msg Message) (Message, error) {
		time.Sleep(2 * time.Second) // 模拟慢节点
		return Message{Type: MsgTypeResult, Content: "late"}, nil
	}))
	rt.Start(context.Background())
	defer rt.Shutdown()

	_, err := rt.Call(context.Background(), "test", "slow", MsgTypeTask, "x", 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "超时") {
		t.Errorf("应返回超时错误, got %v", err)
	}
}
