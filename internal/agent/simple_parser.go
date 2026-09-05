package agent

import (
	"regexp"
	"strconv"
	"strings"
)

// parseTextFallback 无 LLM 时的降级解析：关键词/正则规则匹配。
// 保证未配置模型密钥时接口仍可用（精度低于 LLM 解析）。
func (p *ParserAgent) parseTextFallback(text string) *ParseResult {
	profile := &UserProfile{
		Education:           matchEducation(text),
		PoliticalStatus:     matchPolitical(text),
		IsFreshGraduate:     matchFreshGrad(text),
		TargetProvinces:     matchProvinces(text),
		WorkExperienceYears: matchWorkYears(text),
		Major:               matchMajor(text),
	}
	profile.MajorCategory = majorCategoryOf(profile.Major)

	return &ParseResult{
		Status:          StatusSuccess,
		Profile:         profile,
		Confidence:      0.9,
		UncertainFields: []UncertainField{},
		ParsedFrom:      "text",
	}
}

// matchFreshGrad 判断应届身份：明确提及返回对应指针，未提及返回 nil（未知）。
func matchFreshGrad(text string) *bool {
	if strings.Contains(text, "应届") {
		return boolPtr(true)
	}
	// "毕业N年"明确为非应届
	if gradYearsRe.MatchString(text) {
		return boolPtr(false)
	}
	return nil
}

// boolPtr 返回布尔指针（应届身份三态：true/false/nil）。
func boolPtr(v bool) *bool { return &v }

// gradYearsRe 匹配"毕业N年"表述。
var gradYearsRe = regexp.MustCompile(`毕业\s*\d+\s*年`)

// parseImageFallback 无视觉模型时的 Mock 结果，用于演示 need_confirm 确认流程。
// TODO: 配置视觉模型（如 qwen-vl-max / 本地 Qwen2.5-VL）后走真实图片解析。
func (p *ParserAgent) parseImageFallback() *ParseResult {
	return &ParseResult{
		Status: StatusNeedConfirm,
		ProfilePartial: &UserProfile{
			Education:       "本科",
			TargetProvinces: []string{"广东"},
		},
		Confidence: 0.62,
		UncertainFields: []UncertainField{
			{
				Field:          "major",
				RawText:        "计箅机科?技术",
				Confidence:     0.62,
				SuggestedValue: "计算机科学与技术",
				Reason:         "OCR识别到疑似错别字，根据上下文推断",
			},
			{
				Field:      "political_status",
				RawText:    "",
				Confidence: 0.0,
				Reason:     "图片中未找到政治面貌相关信息",
			},
		},
		ParsedFrom: "image",
	}
}

// matchEducation 匹配学历关键词（从高到低，避免"研究生"被"本科"干扰）。
func matchEducation(text string) string {
	rules := []struct {
		words []string
		value string
	}{
		{[]string{"博士"}, "博士"},
		{[]string{"硕士", "研究生"}, "硕士"},
		{[]string{"本科"}, "本科"},
		{[]string{"大专", "专科"}, "大专"},
	}
	for _, r := range rules {
		for _, w := range r.words {
			if strings.Contains(text, w) {
				return r.value
			}
		}
	}
	return ""
}

// matchPolitical 匹配政治面貌（"预备党员"需优先于"党员"匹配）。
func matchPolitical(text string) string {
	switch {
	case strings.Contains(text, "预备党员"):
		return "预备党员"
	case strings.Contains(text, "党员"):
		return "中共党员"
	case strings.Contains(text, "团员"):
		return "共青团员"
	case strings.Contains(text, "群众"):
		return "群众"
	}
	return ""
}

// provinceNames 省级行政区列表，用于目标省份匹配。
var provinceNames = []string{
	"北京", "天津", "上海", "重庆",
	"广东", "江苏", "浙江", "山东", "河南", "河北", "湖南", "湖北",
	"四川", "福建", "安徽", "江西", "陕西", "山西", "广西", "云南",
	"贵州", "辽宁", "吉林", "黑龙江", "甘肃", "海南", "宁夏", "青海",
	"新疆", "西藏", "内蒙古",
}

// cityProvinceMap 常见城市→省份映射（PRD 6.2："广州"→"广东"）。
var cityProvinceMap = map[string]string{
	"广州": "广东", "深圳": "广东", "珠海": "广东", "佛山": "广东",
	"杭州": "浙江", "宁波": "浙江",
	"南京": "江苏", "苏州": "江苏",
	"成都": "四川", "武汉": "湖北", "长沙": "湖南",
	"西安": "陕西", "青岛": "山东", "济南": "山东",
	"厦门": "福建", "福州": "福建", "合肥": "安徽", "郑州": "河南",
}

// matchProvinces 提取文本中的省份（去重）；城市名映射为所属省份；"国考/国家/中央"映射为"国家"。
func matchProvinces(text string) []string {
	var result []string
	seen := make(map[string]bool)
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	for _, p := range provinceNames {
		if strings.Contains(text, p) {
			add(p)
		}
	}
	for city, province := range cityProvinceMap {
		if strings.Contains(text, city) {
			add(province)
		}
	}
	for _, w := range []string{"国考", "国家", "中央"} {
		if strings.Contains(text, w) {
			add("国家")
			break
		}
	}
	return result
}

// workYearsRe 匹配基层工作经验年数，如"基层工作2年"。
var workYearsRe = regexp.MustCompile(`基层[^0-9]{0,4}(\d+)\s*年`)

// matchWorkYears 匹配基层工作经验年数。
func matchWorkYears(text string) int {
	if m := workYearsRe.FindStringSubmatch(text); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 0
}

// majorKeywords 常见专业关键词（MVP 阶段），后续由专业目录库（major_directory 表）替代。
var majorKeywords = []string{
	"计算机科学与技术", "软件工程", "计算机", "法学", "会计学", "财务管理",
	"汉语言文学", "经济学", "金融学", "工商管理", "行政管理", "土木工程",
	"机械工程", "电气工程", "英语", "新闻学", "统计学", "数学",
}

// majorCategoryMap 专业→专业大类映射（MVP 内置，后续迁移到专业目录库）。
var majorCategoryMap = map[string]string{
	"计算机科学与技术": "计算机类",
	"软件工程":     "计算机类",
	"计算机":      "计算机类",
	"法学":       "法学类",
	"会计学":      "工商管理类",
	"财务管理":     "工商管理类",
	"汉语言文学":    "中国语言文学类",
}

// majorExprRe 匹配"专业是XX""专业：XX"表述中的专业名。
var majorExprRe = regexp.MustCompile(`专业[是：:]\s*([一-龥A-Za-z]+)`)

// matchMajor 提取专业名称：优先"专业是XX"表述，其次关键词表（长词优先）。
func matchMajor(text string) string {
	if m := majorExprRe.FindStringSubmatch(text); len(m) == 2 {
		return m[1]
	}
	for _, major := range majorKeywords {
		if strings.Contains(text, major) {
			return major
		}
	}
	return ""
}

// majorCategoryOf 查询专业所属大类，未收录返回空串。
func majorCategoryOf(major string) string {
	return majorCategoryMap[major]
}
