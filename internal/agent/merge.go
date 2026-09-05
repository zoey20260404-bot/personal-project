package agent

import (
	"fmt"

	"ai-start/internal/types"
)

// mergeProfiles 多源融合：将文本解析结果与毕业证识别结果合并为一份用户画像。
//
// 融合规则（代码规则层，不让 LLM 做判定）：
//   - 证件优先：education / major 以毕业证识别为准（权威来源）
//   - 冲突检测：文本自述与证件识别不一致 → 加入 uncertain_fields 走确认流程
//   - 互补合并：其余字段谁有谁填，省份取并集
//   - 三态字段（is_fresh_graduate）：任一来源有确定值即采用；两来源冲突 → 不确定
func mergeProfiles(textProfile *types.UserProfile, imageProfiles []*types.UserProfile) (*types.UserProfile, []types.UncertainField) {
	merged := &types.UserProfile{}
	var uncertain []types.UncertainField

	// 收集证件来源的权威值（取第一个非空的证件结果）
	var diplomaEdu, diplomaMajor string
	for _, img := range imageProfiles {
		if diplomaEdu == "" {
			diplomaEdu = img.Education
		}
		if diplomaMajor == "" {
			diplomaMajor = img.Major
		}
	}

	// 学历：证件优先，冲突标记
	merged.Education = mergeAuthoritative(textProfile.Education, diplomaEdu, "education", &uncertain)
	// 专业：证件优先，冲突标记
	merged.Major = mergeAuthoritative(textProfile.Major, diplomaMajor, "major", &uncertain)
	if merged.MajorCategory == "" && merged.Major != "" {
		merged.MajorCategory = majorCategoryOf(merged.Major)
	}
	if merged.MajorCategory == "" {
		merged.MajorCategory = textProfile.MajorCategory
	}

	// 互补字段：文本优先，证件补充
	merged.PoliticalStatus = firstNonEmpty(textProfile.PoliticalStatus, pickFromImages(imageProfiles, func(p *types.UserProfile) string { return p.PoliticalStatus }))
	merged.Gender = firstNonEmpty(textProfile.Gender, pickFromImages(imageProfiles, func(p *types.UserProfile) string { return p.Gender }))
	if merged.Age == 0 {
		merged.Age = textProfile.Age
	}
	merged.WorkExperienceYears = textProfile.WorkExperienceYears

	// 省份：并集
	merged.TargetProvinces = unionStrings(textProfile.TargetProvinces, collectImageProvinces(imageProfiles))

	// 应届身份（三态）：文本优先，证件补充，冲突标记
	merged.IsFreshGraduate = mergeFreshGraduate(textProfile.IsFreshGraduate, pickFreshFromImages(imageProfiles), &uncertain)

	// 其他要求：并集
	merged.OtherRequirements = unionStrings(textProfile.OtherRequirements, nil)

	return merged, uncertain
}

// mergeAuthoritative 权威字段合并：证件值优先；文本值与证件值冲突时标记不确定。
func mergeAuthoritative(textVal, diplomaVal, field string, uncertain *[]types.UncertainField) string {
	if diplomaVal == "" {
		return textVal // 无证件值，用文本自述
	}
	if textVal == "" || textVal == diplomaVal {
		return diplomaVal
	}
	// 冲突：采用证件值，但标记不确定让用户确认
	*uncertain = append(*uncertain, types.UncertainField{
		Field:          field,
		RawText:        textVal,
		Confidence:     0.5,
		SuggestedValue: diplomaVal,
		Reason:         fmt.Sprintf("文本自述「%s」与证件识别「%s」不一致，请确认", textVal, diplomaVal),
	})
	return diplomaVal
}

// mergeFreshGraduate 三态字段合并：任一来源有值即采用；两来源冲突标记不确定。
func mergeFreshGraduate(textVal, imgVal *bool, uncertain *[]types.UncertainField) *bool {
	if textVal == nil {
		return imgVal
	}
	if imgVal == nil || *textVal == *imgVal {
		return textVal
	}
	// 冲突：采用文本自述，标记确认
	*uncertain = append(*uncertain, types.UncertainField{
		Field:          "is_fresh_graduate",
		RawText:        fmt.Sprintf("文本=%v", *textVal),
		Confidence:     0.5,
		SuggestedValue: fmt.Sprintf("证件=%v", *imgVal),
		Reason:         "文本自述与证件识别的应届身份冲突，请确认",
	})
	return textVal
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// pickFromImages 从证件识别结果中取第一个非空字段值。
func pickFromImages(images []*types.UserProfile, pick func(*types.UserProfile) string) string {
	for _, img := range images {
		if v := pick(img); v != "" {
			return v
		}
	}
	return ""
}

// pickFreshFromImages 从证件识别结果中取第一个非 nil 的应届值。
func pickFreshFromImages(images []*types.UserProfile) *bool {
	for _, img := range images {
		if img.IsFreshGraduate != nil {
			return img.IsFreshGraduate
		}
	}
	return nil
}

// collectImageProvinces 汇总证件识别出的省份。
func collectImageProvinces(images []*types.UserProfile) []string {
	var out []string
	for _, img := range images {
		out = append(out, img.TargetProvinces...)
	}
	return out
}

// unionStrings 字符串切片并集（去重保序）。
func unionStrings(a, b []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
