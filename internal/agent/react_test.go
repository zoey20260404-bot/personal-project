package agent

import (
	"context"
	"errors"
	"testing"

	"ai-start/internal/llm"
	"ai-start/internal/tool"
)

// fakeLLMCaller 模拟模型调用：按预设脚本依次返回响应（用于 ReAct 循环测试）。
type fakeLLMCaller struct {
	responses []*llm.Message // 依次返回的响应
	calls     int            // 实际调用次数
}

func (f *fakeLLMCaller) ChatCompletion(_ context.Context, _ string, msgs []llm.Message, _ *llm.Options) (*llm.Message, error) {
	f.calls++
	if f.calls > len(f.responses) {
		return nil, errors.New("超出预设响应数量")
	}
	return f.responses[f.calls-1], nil
}

// testToolRegistry 构造含计数工具的注册表。
func testToolRegistry(execResult string, execCount *int) *tool.Registry {
	reg := tool.NewRegistry()
	reg.Register(&countingTool{result: execResult, count: execCount})
	return reg
}

// countingTool 记录执行次数的测试工具。
type countingTool struct {
	result string
	count  *int
}

func (t *countingTool) Name() string        { return "mock_tool" }
func (t *countingTool) Description() string { return "测试工具" }
func (t *countingTool) ParamsJSON() string  { return `{"type":"object","properties":{}}` }
func (t *countingTool) Execute(_ context.Context, _ string) (string, error) {
	*t.count++
	return t.result, nil
}

// TestReActToolLoop 验证 ReAct 循环：模型先发起工具调用，拿到结果后给出最终回答。
func TestReActToolLoop(t *testing.T) {
	var execCount int
	caller := &fakeLLMCaller{responses: []*llm.Message{
		// 第 1 步：模型要求调用工具
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "mock_tool", Arguments: "{}"}}},
		// 第 2 步：模型基于工具结果给出最终回答
		{Role: "assistant", Content: "最终回答"},
	}}
	a := NewReActAgent("test", "chat", nil, "", 5, nil, caller, testToolRegistry("工具结果", &execCount), []string{"mock_tool"}, nil, nil)

	answer, err := a.Run(context.Background(), "用户问题")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if answer != "最终回答" {
		t.Errorf("回答错误: got %q", answer)
	}
	if execCount != 1 {
		t.Errorf("工具应执行 1 次，实际 %d 次", execCount)
	}
	if caller.calls != 2 {
		t.Errorf("模型应调用 2 次，实际 %d 次", caller.calls)
	}
}

// TestReActMaxSteps 验证达到最大步数上限时返回错误（防止死循环）。
func TestReActMaxSteps(t *testing.T) {
	var execCount int
	caller := &fakeLLMCaller{responses: []*llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "mock_tool", Arguments: "{}"}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c2", Name: "mock_tool", Arguments: "{}"}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c3", Name: "mock_tool", Arguments: "{}"}}},
	}}
	a := NewReActAgent("test", "chat", nil, "", 3, nil, caller, testToolRegistry("工具结果", &execCount), []string{"mock_tool"}, nil, nil)

	if _, err := a.Run(context.Background(), "用户问题"); err == nil {
		t.Error("达到最大步数应返回错误")
	}
	if caller.calls != 3 {
		t.Errorf("模型应调用 3 次（max_steps），实际 %d 次", caller.calls)
	}
}
