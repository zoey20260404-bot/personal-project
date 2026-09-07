package agent

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	config "ai-start/configs"
	"ai-start/internal/llm"
	"ai-start/internal/prompt"
	"ai-start/internal/tool"
)

// TestInterviewerConfiguration 验证最新示例配置可以通过原有装配逻辑注册面试节点。
func TestInterviewerConfiguration(t *testing.T) {
	cfg, err := config.Load("../../configs/config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ac, ok := cfg.Agents["interviewer"]
	if !ok || ac.Type != AgentTypeReact || len(ac.Tools) != 0 {
		t.Fatal("面试节点必须是无工具 ReAct")
	}
	for _, mode := range []string{"beginner", "advanced"} {
		text, err := prompt.NewStore().Render(ac.Prompt, map[string]any{"Mode": mode})
		if err != nil || text == "" || strings.Contains(text, "{{") {
			t.Fatalf("Prompt 渲染失败：%v", err)
		}
	}
	rt, err := BuildRuntime(cfg.Agents, cfg.Runtime, nil, prompt.NewStore(), tool.NewRegistry(), slog.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.HasNode("interviewer") || !rt.HasNode("advisor") || !rt.HasNode("router") {
		t.Fatal("chat 节点装配不完整")
	}
}

// TestInterviewerRejectsTools 模型伪造全局已注册工具时，空白名单仍阻止执行。
func TestInterviewerRejectsTools(t *testing.T) {
	var executions int
	caller := &fakeLLMCaller{responses: []*llm.Message{{
		Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "fake", Name: "mock_tool", Arguments: "{}"}},
	}}}
	a := NewReActAgent("interviewer", "chat", prompt.NewStore(), prompt.NameInterviewer, 1, nil, caller, testToolRegistry("不应调用", &executions), nil, nil, nil)
	_, err := a.Run(context.Background(), "模拟面试")
	if err == nil || !strings.Contains(err.Error(), "无权调用") || executions != 0 {
		t.Fatalf("工具隔离失败：%v，执行次数 %d", err, executions)
	}
}
