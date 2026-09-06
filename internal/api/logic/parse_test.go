package logic

import (
	"testing"

	"ai-start/internal/types"
)

// TestMergeWithExisting 验证档案合并：未确认/空缺字段保留历史值，确定字段更新。
func TestMergeWithExisting(t *testing.T) {
	freshFalse := false
	old := &types.UserProfile{
		Education:       "本科",
		Major:           "计算机科学与技术",
		MajorCategory:   "计算机类",
		PoliticalStatus: "群众",
		IsFreshGraduate: &freshFalse,
		TargetProvinces: []string{"广东"},
	}
	freshTrue := true
	newProfile := &types.UserProfile{
		Education:       "硕士",   // 新值确定 → 更新
		Major:           "",     // 空缺 → 保留历史
		PoliticalStatus: "中共党员", // 在不确定列表 → 保留历史
		IsFreshGraduate: &freshTrue,
		TargetProvinces: []string{"湖南"},
	}
	uncertain := []types.UncertainField{{Field: types.FieldPoliticalStatus}}

	merged := mergeWithExisting(newProfile, uncertain, old)

	if merged.Education != "硕士" {
		t.Errorf("确定字段应更新: got %q", merged.Education)
	}
	if merged.Major != "计算机科学与技术" {
		t.Errorf("空缺字段应保留历史值: got %q", merged.Major)
	}
	if merged.PoliticalStatus != "群众" {
		t.Errorf("未确认字段应保留历史值: got %q", merged.PoliticalStatus)
	}
	if merged.IsFreshGraduate == nil || !*merged.IsFreshGraduate {
		t.Error("确定的应届字段应更新")
	}
	if len(merged.TargetProvinces) != 1 || merged.TargetProvinces[0] != "湖南" {
		t.Errorf("确定的省份应更新: got %v", merged.TargetProvinces)
	}
	// 历史对象不应被修改
	if old.Education != "本科" {
		t.Error("mergeWithExisting 不应修改历史档案对象")
	}
}

// TestMergeWithExistingNilNew 验证新结果为空时返回历史档案。
func TestMergeWithExistingNilNew(t *testing.T) {
	old := &types.UserProfile{Education: "本科"}
	merged := mergeWithExisting(nil, nil, old)
	if merged != old {
		t.Error("新结果为空应直接返回历史档案")
	}
}
