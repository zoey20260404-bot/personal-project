package logic

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"ai-start/internal/runtime"
	"ai-start/internal/store"
)

// TestChatInterviewContext 验证完整上下文经过现有总线交给 Router 和目标节点。
// 路由桩只验证编排，真实模型的语义判断需要单独交互验收。
func TestChatInterviewContext(t *testing.T) {
	ctx := context.Background()
	buffer := store.NewMemoryChatBuffer()
	_ = buffer.Append(ctx, "session", "assistant", "模拟模式 · 第 1/3 题：如何处理群众投诉？")
	for i := 0; i < 10; i++ {
		_ = buffer.Append(ctx, "session", "user", "补充作答")
	}
	rt := runtime.NewRuntime(16, slog.Default())
	routerInputs, targetInputs := make(chan string, 2), make(chan string, 2)
	rt.Register(runtime.NewNode("router", 4, func(_ context.Context, msg runtime.Message) (runtime.Message, error) {
		routerInputs <- msg.Content
		target := "interviewer"
		if strings.HasSuffix(msg.Content, "【当前问题】\n帮我查岗位") {
			target = "advisor"
		}
		return runtime.Message{Content: `{"target":"` + target + `","confidence":0.95}`}, nil
	}))
	for _, name := range []string{"interviewer", "advisor"} {
		rt.Register(runtime.NewNode(name, 4, func(_ context.Context, msg runtime.Message) (runtime.Message, error) {
			targetInputs <- msg.Content
			return runtime.Message{Content: "收到"}, nil
		}))
	}
	rt.Start(ctx)
	defer rt.Shutdown()
	svc := NewChatService(rt, buffer, NewParseService(nil, nil, slog.Default(), nil), slog.Default())
	for _, tc := range []struct{ question, target string }{
		{"我先安抚群众，再核实情况", "interviewer"},
		{"帮我查岗位", "advisor"},
	} {
		reply, err := svc.Chat(ctx, 1, "session", tc.question, "beginner", nil)
		if err != nil {
			t.Fatal(err)
		}
		if reply.Agent != tc.target || reply.Answer != "收到" {
			t.Fatalf("错误回复：%+v", reply)
		}
		routed, received := <-routerInputs, <-targetInputs
		if routed != received || !strings.Contains(routed, "如何处理群众投诉") || !strings.HasSuffix(routed, tc.question) {
			t.Fatalf("上下文缺失：%q / %q", routed, received)
		}
	}
}
