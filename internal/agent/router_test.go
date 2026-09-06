package agent

import (
	"context"
	"testing"
)

// TestRouterFallbackNoLLM 验证 LLM 不可用时路由兜底（不报错，返回兜底目标）。
func TestRouterFallbackNoLLM(t *testing.T) {
	r := NewRouterAgent(nil, "", nil, "advisor")
	result := r.Route(context.Background(), "帮我看看广东有什么岗")
	if result.Target != "advisor" || result.Confidence != 1.0 {
		t.Errorf("兜底路由错误: %+v", result)
	}
}

// TestParseRoute 验证路由输出解析（正常/低置信度澄清/非法输出兜底）。
func TestParseRoute(t *testing.T) {
	r := NewRouterAgent(nil, "", nil, "advisor")

	// 正常路由
	res := r.parseRoute(`{"agent":"interviewer","confidence":0.92,"reason":"面试模拟"}`)
	if res.Target != "interviewer" || res.Clarify {
		t.Errorf("正常路由解析错误: %+v", res)
	}

	// 低置信度 → 澄清
	res = r.parseRoute(`{"agent":"advisor","confidence":0.5,"reason":"不确定"}`)
	if !res.Clarify {
		t.Error("低置信度应触发澄清")
	}

	// 非法输出 → 兜底
	res = r.parseRoute("我不知道怎么选")
	if res.Target != "advisor" {
		t.Errorf("非法输出应兜底 advisor, got %q", res.Target)
	}

	// confidence 为字符串的容错
	res = r.parseRoute(`{"agent":"exam_coach","confidence":"0.9","reason":"笔试"}`)
	if res.Target != "exam_coach" || res.Confidence != 0.9 {
		t.Errorf("字符串置信度容错失败: %+v", res)
	}
}
