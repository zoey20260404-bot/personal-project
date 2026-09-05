package agent

import (
	"testing"

	"ai-start/internal/types"
)

// TestValidateProvinceNormalization 验证城市名归一为省份（问题 1 回归）。
func TestValidateProvinceNormalization(t *testing.T) {
	result := &ParseResult{
		Status: StatusSuccess,
		Profile: &types.UserProfile{
			Education:       "本科",
			Major:           "计算机",
			PoliticalStatus: "中共党员",
			IsFreshGraduate: boolPtr(true),
			// LLM 误输出城市名 + 一个非法值
			TargetProvinces: []string{"广州", "深圳", "广东", "火星"},
		},
		Confidence: 0.5, // LLM 惯性低报，应被规则值取代
	}
	out := validateResult(result)

	// 广州/深圳 → 广东（去重），"火星"剔除；归一后省份有效 → success
	if out.Status != StatusSuccess {
		t.Errorf("归一后省份有效，应为 success, got %q", out.Status)
	}
	if len(out.Profile.TargetProvinces) != 1 || out.Profile.TargetProvinces[0] != "广东" {
		t.Errorf("省份归一错误: got %v, want [广东]", out.Profile.TargetProvinces)
	}
	if len(out.UncertainFields) != 0 {
		t.Errorf("省份归一有效时不确定项应被清理, got %v", out.UncertainFields)
	}
	// 无剩余不确定项时置信度为规则值 1.0
	if out.Confidence != 1.0 {
		t.Errorf("无不确定项时置信度应为规则值 1.0, got %v", out.Confidence)
	}
}

// TestValidateAllProvincesInvalid 验证省份全部非法时：字段无有效值，保留不确定项并触发确认。
func TestValidateAllProvincesInvalid(t *testing.T) {
	result := &ParseResult{
		Status: StatusSuccess,
		Profile: &types.UserProfile{
			Education:       "本科",
			Major:           "计算机",
			PoliticalStatus: "中共党员",
			IsFreshGraduate: boolPtr(true),
			TargetProvinces: []string{"火星"}, // 全部非法
		},
		Confidence: 0.9,
	}
	out := validateResult(result)

	if out.Status != StatusNeedConfirm {
		t.Errorf("省份全部非法应触发 need_confirm, got %q", out.Status)
	}
	found := false
	for _, uf := range out.UncertainFields {
		if uf.Field == "target_provinces" {
			found = true
		}
	}
	if !found {
		t.Error("省份全部非法时应保留 target_provinces 不确定项")
	}
}

// TestValidateFreshNullTriggersConfirm 验证应届字段未提及（null）触发确认（问题 2 回归）。
func TestValidateFreshNullTriggersConfirm(t *testing.T) {
	result := &ParseResult{
		Status: StatusSuccess,
		Profile: &types.UserProfile{
			Education:       "本科",
			Major:           "计算机",
			PoliticalStatus: "中共党员",
			IsFreshGraduate: nil, // 未提及
			TargetProvinces: []string{"广东"},
		},
		Confidence: 0.95, // LLM 虚报的高置信度
	}
	out := validateResult(result)

	if out.Status != StatusNeedConfirm {
		t.Errorf("应届未知应触发 need_confirm, got %q", out.Status)
	}
	if out.Profile != nil || out.ProfilePartial == nil {
		t.Error("need_confirm 时应返回 profile_partial")
	}
	// 置信度规则重算：1 - 0.15*1 = 0.85
	if out.Confidence > 0.85 {
		t.Errorf("置信度应被规则重算（≤0.85）, got %v", out.Confidence)
	}
}

// TestValidateContradictoryUncertain 验证矛盾清理：字段已有确定值却仍被列为不确定时，应移除（不再触发确认）。
// 回归场景：用户输入"工作2~3年"，LLM 解析出应届=否，却又把应届身份列入 uncertain_fields。
func TestValidateContradictoryUncertain(t *testing.T) {
	result := &ParseResult{
		Status: StatusSuccess,
		Profile: &types.UserProfile{
			Education:       "本科",
			Major:           "软件工程",
			PoliticalStatus: "中共党员",
			IsFreshGraduate: boolPtr(false), // 已从"工作2~3年"确定为非应届
			TargetProvinces: []string{"广东"},
		},
		Confidence: 0.9,
		UncertainFields: []types.UncertainField{
			// LLM 矛盾输出：字段已有值，却仍标记不确定
			{Field: "is_fresh_graduate", Confidence: 0.0, Reason: "用户未提及应届身份"},
			// 非核心可选字段不应触发确认
			{Field: "gender", Confidence: 0.0, Reason: "用户未提及性别"},
			{Field: "age", Confidence: 0.0, Reason: "用户未提及年龄"},
		},
	}
	out := validateResult(result)

	if len(out.UncertainFields) != 0 {
		t.Errorf("矛盾/非核心不确定项应被清理, got %v", out.UncertainFields)
	}
	if out.Status != StatusSuccess {
		t.Errorf("字段完整应为 success, got %q", out.Status)
	}
}

// TestValidateCompleteProfile 验证字段完整时保持 success 且置信度不被压低。
func TestValidateCompleteProfile(t *testing.T) {
	result := &ParseResult{
		Status: StatusSuccess,
		Profile: &types.UserProfile{
			Education:       "本科",
			Major:           "计算机科学与技术",
			PoliticalStatus: "中共党员",
			IsFreshGraduate: boolPtr(true),
			TargetProvinces: []string{"广东"},
		},
		Confidence: 0.95, // LLM 自报值将被规则置信度取代
	}
	out := validateResult(result)

	if out.Status != StatusSuccess {
		t.Errorf("字段完整应为 success, got %q", out.Status)
	}
	if out.Confidence != 1.0 {
		t.Errorf("完整字段置信度应为规则值 1.0, got %v", out.Confidence)
	}
}
