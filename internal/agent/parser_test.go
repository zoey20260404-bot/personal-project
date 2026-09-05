package agent

import "testing"

// TestParseTextFallback 验证无 LLM 时的降级解析（PRD 7.1 验收示例）。
func TestParseTextFallback(t *testing.T) {
	p := NewParserAgent(nil, "", nil, "")
	result := p.parseTextFallback("我是计算机本科，党员，想考广州")

	if result.Status != StatusSuccess {
		t.Fatalf("状态错误: got %q, want %q", result.Status, StatusSuccess)
	}
	profile := result.Profile
	if profile.Education != "本科" {
		t.Errorf("学历解析错误: got %q, want 本科", profile.Education)
	}
	if profile.PoliticalStatus != "中共党员" {
		t.Errorf("政治面貌解析错误: got %q, want 中共党员", profile.PoliticalStatus)
	}
	if len(profile.TargetProvinces) != 1 || profile.TargetProvinces[0] != "广东" {
		t.Errorf("省份解析错误: got %v, want [广东]", profile.TargetProvinces)
	}
	if profile.Major != "计算机" {
		t.Errorf("专业解析错误: got %q, want 计算机", profile.Major)
	}
	if profile.MajorCategory != "计算机类" {
		t.Errorf("专业大类映射错误: got %q, want 计算机类", profile.MajorCategory)
	}
}

// TestMajorCategoryMapping 验证专业大类映射（PRD 7.1："软件工程"→"计算机类"）。
func TestMajorCategoryMapping(t *testing.T) {
	if got := majorCategoryOf("软件工程"); got != "计算机类" {
		t.Errorf("软件工程大类映射错误: got %q, want 计算机类", got)
	}
}

// TestExtractJSON 验证从 LLM 输出中提取 JSON（含 markdown 代码块场景）。
func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"纯JSON", `{"a":1}`, `{"a":1}`},
		{"代码块包裹", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"带多余文字", "好的，结果如下：{\"a\":1} 以上", `{"a":1}`},
	}
	for _, tc := range cases {
		if got := extractJSON(tc.input); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestBuildResultNeedConfirm 验证低置信度字段触发 need_confirm（PRD 2.1 规则）。
func TestBuildResultNeedConfirm(t *testing.T) {
	p := NewParserAgent(nil, "", nil, "")
	raw := `{"profile":{"education":"本科","major":""},"confidence":0.9,"uncertain_fields":[{"field":"major","raw_text":"计箅机","confidence":0.62,"suggested_value":"计算机科学与技术","reason":"疑似错别字"}]}`
	result, err := p.buildResult(raw, "text")
	if err != nil {
		t.Fatalf("buildResult 失败: %v", err)
	}
	if result.Status != StatusNeedConfirm {
		t.Errorf("状态错误: got %q, want %q", result.Status, StatusNeedConfirm)
	}
	if result.Profile != nil || result.ProfilePartial == nil {
		t.Error("need_confirm 时应返回 profile_partial 而非 profile")
	}
}

// TestConfirm 验证确认修正合并逻辑（PRD 3.2.2）。
func TestConfirm(t *testing.T) {
	partial := &UserProfile{Education: "本科", TargetProvinces: []string{"广东"}}
	merged := Confirm(partial, map[string]string{
		"major":            "计算机科学与技术",
		"political_status": "中共党员",
	})

	if merged.Major != "计算机科学与技术" {
		t.Errorf("专业确认失败: got %q", merged.Major)
	}
	if merged.PoliticalStatus != "中共党员" {
		t.Errorf("政治面貌确认失败: got %q", merged.PoliticalStatus)
	}
	if merged.MajorCategory != "计算机类" {
		t.Errorf("确认后应重新映射专业大类: got %q", merged.MajorCategory)
	}
	// 原对象不应被修改
	if partial.Major != "" {
		t.Error("Confirm 不应修改原始 partial 对象")
	}
}
