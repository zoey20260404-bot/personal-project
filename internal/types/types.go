// Package types 定义跨层共享的数据模型。
// 独立成包以避免 agent 与 store 之间的循环依赖。
package types

// UserProfile 标准化用户条件结构（PRD 3.2.1 响应中的 profile 字段）。
type UserProfile struct {
	Education           string   `json:"education"`             // 学历：大专/本科/硕士/博士
	Major               string   `json:"major"`                 // 专业名称（精确匹配专业目录）
	MajorCategory       string   `json:"major_category"`        // 专业大类，如"计算机类"
	PoliticalStatus     string   `json:"political_status"`      // 政治面貌：群众/共青团员/中共党员/预备党员
	IsFreshGraduate     *bool    `json:"is_fresh_graduate"`     // 是否应届生（nil=未提及/未知，避免模型被迫猜测）
	TargetProvinces     []string `json:"target_provinces"`      // 目标省份（"国家"表示国考；城市须映射为所属省份）
	WorkExperienceYears int      `json:"work_experience_years"` // 基层工作经验年数
	Gender              string   `json:"gender"`                // 性别（部分岗位有限制）
	Age                 int      `json:"age"`                   // 年龄（部分岗位有上限）
	OtherRequirements   []string `json:"other_requirements"`    // 其他要求，如"不接受经常出差"
}

// ImageInput 图片输入（多源融合解析用）。
type ImageInput struct {
	Type   string `json:"type"`   // 图片类型：diploma_image（毕业证/学位证）；position_image 预留给岗位分析
	Base64 string `json:"base64"` // 图片 Base64 编码内容
}

// UncertainField OCR/解析低置信度字段（PRD 2.1 OCR方案C）。
type UncertainField struct {
	Field          string  `json:"field"`           // 字段名
	RawText        string  `json:"raw_text"`        // 识别到的原始文本
	Confidence     float64 `json:"confidence"`      // 置信度 0-1
	SuggestedValue string  `json:"suggested_value"` // 建议值（根据上下文推断）
	Reason         string  `json:"reason"`          // 不确定原因说明
}

// ParseResult 条件解析结果，对应 POST /api/v1/parse 的响应数据。
type ParseResult struct {
	Status          string           `json:"status"`           // success | need_confirm
	Profile         *UserProfile     `json:"profile"`          // 完整条件（status=success 时）
	ProfilePartial  *UserProfile     `json:"profile_partial"`  // 部分条件（status=need_confirm 时）
	Confidence      float64          `json:"confidence"`       // 整体置信度
	UncertainFields []UncertainField `json:"uncertain_fields"` // 待确认字段列表
	SessionID       string           `json:"session_id"`       // 会话 ID（关联用户数据与确认流程）
	ParsedFrom      string           `json:"parsed_from"`      // 解析来源：text / image
}

// 解析状态常量。
const (
	StatusSuccess     = "success"      // 全部字段置信度达标，直接成功
	StatusNeedConfirm = "need_confirm" // 存在低置信度字段，需用户确认
)
