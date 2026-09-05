package agent

import (
	"math"

	"ai-start/internal/types"
)

// validateResult 规则校验层：对 LLM 输出做归一化与置信度重算。
//
// 背景（见 docs/feat001/test-issues.md）：小模型的指令遵循不可靠——
// 城市可能没映射成省份、未提及的字段可能被编造、自报置信度虚高。
// 原则：LLM 负责理解，确定性判定收归代码兜底。
func validateResult(result *ParseResult) *ParseResult {
	profile := result.Profile
	if profile == nil {
		profile = result.ProfilePartial
	}
	if profile == nil {
		return result
	}

	// 已有不确定字段按字段名索引，避免重复
	uncertain := make(map[string]types.UncertainField)
	for _, uf := range result.UncertainFields {
		uncertain[uf.Field] = uf
	}

	// 1. 省份归一：城市→省份，非法值剔除并标记不确定
	profile.TargetProvinces = normalizeProvinces(profile.TargetProvinces, uncertain)

	// 2. 核心字段空缺检查（学历/专业/政治面貌/应届）
	checkCoreFields(profile, uncertain)

	// 3. 清理矛盾/无关的不确定项：
	//    - 字段已有确定值却仍被列为不确定（LLM 自相矛盾，如"应届=否"却又报"未提及"）
	//    - 非核心可选字段（性别/年龄等），未提及不阻塞流程
	pruneUncertainFields(profile, uncertain)

	// 回写不确定字段列表
	result.UncertainFields = result.UncertainFields[:0]
	for _, uf := range uncertain {
		result.UncertainFields = append(result.UncertainFields, uf)
	}

	// 3. 置信度完全由规则计算（不采信 LLM 自报值——清理后无不确定项时，
	//    LLM 可能仍按 Prompt 惯性自报低分，不能据此阻塞流程）
	result.Confidence = math.Max(0, 1.0-0.15*float64(len(result.UncertainFields)))

	// 4. 状态判定：存在不确定字段或规则置信度低于阈值 → need_confirm
	if len(result.UncertainFields) > 0 || result.Confidence < confidenceThreshold {
		result.Status = StatusNeedConfirm
		result.ProfilePartial = profile
		result.Profile = nil
	} else {
		result.Status = StatusSuccess
		result.Profile = profile
		result.ProfilePartial = nil
	}
	return result
}

// normalizeProvinces 省份归一化：城市名映射为所属省份，去重，非法值标记不确定。
func normalizeProvinces(provinces []string, uncertain map[string]types.UncertainField) []string {
	var result []string
	seen := make(map[string]bool)
	for _, p := range provinces {
		if p == "国家" || containsStr(provinceNames, p) {
			if !seen[p] {
				seen[p] = true
				result = append(result, p)
			}
			continue
		}
		// 城市 → 省份映射
		if province, ok := cityProvinceMap[p]; ok {
			if !seen[province] {
				seen[province] = true
				result = append(result, province)
			}
			continue
		}
		// 非法值：剔除并标记
		uncertain["target_provinces"] = types.UncertainField{
			Field:      "target_provinces",
			RawText:    p,
			Confidence: 0.3,
			Reason:     "无法识别的省份/城市，已从目标省份中剔除，请确认",
		}
	}
	return result
}

// checkCoreFields 核心字段空缺检查：学历/专业/政治面貌为空、应届未知时标记不确定。
func checkCoreFields(profile *types.UserProfile, uncertain map[string]types.UncertainField) {
	coreChecks := []struct {
		field  string
		empty  bool
		reason string
	}{
		{"education", profile.Education == "", "未获取到学历信息"},
		{"major", profile.Major == "", "未获取到专业信息"},
		{"political_status", profile.PoliticalStatus == "", "未获取到政治面貌信息"},
		{"is_fresh_graduate", profile.IsFreshGraduate == nil, "用户未提及应届身份"},
	}
	for _, c := range coreChecks {
		if c.empty {
			if _, exists := uncertain[c.field]; !exists {
				uncertain[c.field] = types.UncertainField{
					Field:      c.field,
					Confidence: 0.0,
					Reason:     c.reason,
				}
			}
		}
	}
}

// coreFields 核心字段集合：只有核心字段的不确定才会触发 need_confirm。
var coreFields = map[string]bool{
	"education": true, "major": true, "major_category": true,
	"political_status": true, "is_fresh_graduate": true, "target_provinces": true,
}

// pruneUncertainFields 清理不确定字段列表：
// 1. 字段已有确定值却仍被列为不确定（LLM 自相矛盾）→ 移除；
// 2. 非核心可选字段（性别/年龄等）→ 移除，不阻塞确认流程。
func pruneUncertainFields(profile *types.UserProfile, uncertain map[string]types.UncertainField) {
	for field, uf := range uncertain {
		if !coreFields[field] {
			delete(uncertain, field)
			continue
		}
		_ = uf
		if fieldHasValue(profile, field) {
			delete(uncertain, field)
		}
	}
}

// fieldHasValue 判断核心字段是否已有确定值。
func fieldHasValue(profile *types.UserProfile, field string) bool {
	switch field {
	case "education":
		return profile.Education != ""
	case "major":
		return profile.Major != ""
	case "major_category":
		return profile.MajorCategory != ""
	case "political_status":
		return profile.PoliticalStatus != ""
	case "is_fresh_graduate":
		return profile.IsFreshGraduate != nil
	case "target_provinces":
		return len(profile.TargetProvinces) > 0
	}
	return false
}

// containsStr 判断字符串切片是否包含目标值。
func containsStr(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}
