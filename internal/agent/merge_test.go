package agent

import (
	"testing"

	"ai-start/internal/types"
)

// TestMergeProfilesConflict 验证文本与证件冲突时：采用证件值并标记不确定。
func TestMergeProfilesConflict(t *testing.T) {
	text := &types.UserProfile{Education: "本科", Major: "计算机", PoliticalStatus: "中共党员", TargetProvinces: []string{"广东"}}
	diploma := &types.UserProfile{Education: "硕士", Major: "软件工程"} // 证件与文本冲突

	merged, uncertain := mergeProfiles(text, []*types.UserProfile{diploma})

	// 证件优先
	if merged.Education != "硕士" || merged.Major != "软件工程" {
		t.Errorf("证件值应优先: got edu=%q major=%q", merged.Education, merged.Major)
	}
	// 冲突字段进入不确定列表
	var fields []string
	for _, uf := range uncertain {
		fields = append(fields, uf.Field)
	}
	if len(fields) != 2 {
		t.Errorf("应有 2 个冲突字段（education/major）, got %v", fields)
	}
	// 无冲突字段互补合并
	if merged.PoliticalStatus != "中共党员" || len(merged.TargetProvinces) != 1 {
		t.Errorf("互补字段合并错误: %+v", merged)
	}
}

// TestMergeProfilesConsistent 验证来源一致时无不确定项。
func TestMergeProfilesConsistent(t *testing.T) {
	text := &types.UserProfile{Education: "本科", Major: "软件工程"}
	diploma := &types.UserProfile{Education: "本科", Major: "软件工程"}

	merged, uncertain := mergeProfiles(text, []*types.UserProfile{diploma})

	if len(uncertain) != 0 {
		t.Errorf("一致时不应有不确定项, got %v", uncertain)
	}
	if merged.MajorCategory != "计算机类" {
		t.Errorf("专业大类应自动映射: got %q", merged.MajorCategory)
	}
}

// TestMergeFreshGraduate 验证三态字段合并：冲突标记，单源确定直接采用。
func TestMergeFreshGraduate(t *testing.T) {
	var uncertain []types.UncertainField

	// 单源确定
	if v := mergeFreshGraduate(boolPtr(true), nil, &uncertain); v == nil || !*v {
		t.Error("单源确定应直接采用")
	}
	if len(uncertain) != 0 {
		t.Error("单源确定不应有不确定项")
	}

	// 双源冲突
	uncertain = nil
	v := mergeFreshGraduate(boolPtr(true), boolPtr(false), &uncertain)
	if v == nil || !*v {
		t.Error("冲突时应采用文本自述")
	}
	if len(uncertain) != 1 || uncertain[0].Field != "is_fresh_graduate" {
		t.Errorf("冲突应标记不确定, got %v", uncertain)
	}
}
