package store

import ()

// Position 职位表（PRD 5.1）：历年国考/省考岗位数据。
type Position struct {
	ID                     uint64 `gorm:"primaryKey;autoIncrement"`                   // 自增主键
	ExamType               string `gorm:"column:exam_type;size:20;index"`             // 国考 | 省考
	Year                   int    `gorm:"column:year;index"`                          // 年度，如 2025
	Province               string `gorm:"column:province;size:50;index"`              // 省份（国考为"国家"）
	City                   string `gorm:"column:city;size:50"`                        // 城市
	Department             string `gorm:"column:department;size:200"`                 // 招录单位
	PositionName           string `gorm:"column:position_name;size:200"`              // 岗位名称
	PositionCode           string `gorm:"column:position_code;size:100"`              // 职位代码
	EducationReq           string `gorm:"column:education_req;size:50"`               // 学历要求：大专/本科/硕士/博士/不限
	MajorReqExact          string `gorm:"column:major_req_exact;type:text"`           // 精确专业要求，逗号分隔
	MajorReqCategory       string `gorm:"column:major_req_category;size:100"`         // 专业大类，如"计算机类"
	PoliticalReq           string `gorm:"column:political_req;size:50"`               // 政治面貌要求：群众/共青团员/中共党员/不限
	FreshGraduateReq       *bool  `gorm:"column:fresh_graduate_req"`                  // 是否限应届（nil=不限）
	WorkExperienceYearsReq int    `gorm:"column:work_experience_years_req;default:0"` // 基层工作年限要求
	GenderReq              string `gorm:"column:gender_req;size:10"`                  // 男 | 女 | 不限
	AgeLimit               int    `gorm:"column:age_limit"`                           // 年龄上限
	Remarks                string `gorm:"column:remarks;type:text"`                   // 岗位备注（暗坑提示来源）
	Score2025              int    `gorm:"column:score_2025"`                          // 2025 进面最低分
	Score2024              int    `gorm:"column:score_2024"`                          // 2024 进面最低分
	ApplicantRatio2025     string `gorm:"column:applicant_ratio_2025;size:20"`        // 2025 报录比，如 "1:85"
}

// TableName 指定表名。
func (Position) TableName() string { return "positions" }

// PositionFilter 岗位查询条件（全部可选，零值不生效）。
type PositionFilter struct {
	ExamType string // 国考 | 省考
	Province string // 单个省份（手动筛选用，优先于 Provinces）
	City     string // 城市
	Keyword  string // 关键词（岗位名/单位模糊匹配）
	// 用户条件（Researcher 硬条件过滤，PRD 2.2）
	Education     string   // 用户学历（岗位学历要求 ≤ 用户学历等级）
	MajorCategory string   // 用户专业大类（岗位大类匹配或不限）
	Political     string   // 用户政治面貌（满足岗位要求）
	IsFresh       *bool    // 用户应届身份
	Provinces     []string // 目标省份多值（档案条件，IN 查询）
	Page          int      // 页码（1 起）
	PageSize      int      // 每页条数
}

// educationRank 学历等级映射（用于"岗位要求 ≤ 用户学历"比较）。
var educationRank = map[string]int{
	"不限": 0, "大专": 1, "本科": 2, "硕士": 3, "博士": 4,
}

// QueryPositions 按条件分页查询岗位。
// 硬条件匹配规则（PRD 2.2，纯规则无 AI）：
//   - 学历：岗位学历要求等级 ≤ 用户学历等级（"不限"所有人可报）
//   - 政治面貌：用户满足岗位要求（党员可报"党员/不限"，团员可报"团员/不限"，群众只能报"不限"）
//   - 应届：岗位限应届时用户必须是应届；用户非应届时不能报限应届岗
func (s *MySQLStore) QueryPositions(filter PositionFilter) ([]Position, int64, error) {
	query := s.db.Model(&Position{})

	if filter.ExamType != "" {
		query = query.Where("exam_type = ?", filter.ExamType)
	}
	if filter.Province != "" {
		if filter.Province == "国家" {
			query = query.Where("province = ?", filter.Province)
		} else {
			// 省份过滤不排除国考（国考 province 为"国家"，但岗位工作地可能在用户目标省份，
			// 如"国家税务总局广东省税务局·广州"）。展示规则：不限定地区的岗 + 符合目标省份的岗。
			query = query.Where("province = ? OR province = '国家'", filter.Province)
		}
	} else if len(filter.Provinces) > 0 {
		// 多目标省份（档案条件）：IN 目标省份 + 国考
		query = query.Where("province IN ? OR province = '国家'", filter.Provinces)
	}
	if filter.City != "" {
		query = query.Where("city = ?", filter.City)
	}
	if filter.Keyword != "" {
		like := "%" + filter.Keyword + "%"
		query = query.Where("position_name LIKE ? OR department LIKE ?", like, like)
	}
	// 学历：岗位要求等级 ≤ 用户等级（含多值表述，见 educationRankSQL）
	if rank, ok := educationRank[filter.Education]; ok {
		query = query.Where(educationRankSQL(), rank)
	}
	// 专业大类：岗位不限 或 岗位要求的多个大类中包含用户大类（"计算机类、电子信息类"这类多值）
	if filter.MajorCategory != "" {
		query = query.Where("major_req_category = '' OR major_req_category = '不限' OR major_req_category LIKE ?", "%"+filter.MajorCategory+"%")
	}
	// 政治面貌：岗位不限 或 岗位要求的多值中包含用户面貌（如"中共党员或共青团员"）
	if filter.Political != "" {
		query = query.Where("political_req = '' OR political_req = '不限' OR political_req LIKE ?", "%"+filter.Political+"%")
	}
	// 应届：限应届岗只有应届可报
	if filter.IsFresh != nil && !*filter.IsFresh {
		query = query.Where("fresh_graduate_req IS NULL OR fresh_graduate_req = false")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, pageSize := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	var positions []Position
	err := query.Order("year DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&positions).Error
	return positions, total, err
}

// educationRankSQL 学历最低要求等级 SQL：岗位要求可能为多值/区间表述
// （"本科及以上""本科或硕士""仅限本科"），按出现的最低等级取值（含大专→1，含本科→2……）。
func educationRankSQL() string {
	return `CASE
		WHEN education_req LIKE '%大专%' THEN 1
		WHEN education_req LIKE '%本科%' THEN 2
		WHEN education_req LIKE '%硕士%' THEN 3
		WHEN education_req LIKE '%博士%' THEN 4
		ELSE 0
	END <= ?`
}

// SeedPositions 表为空时写入示例数据（开发演示用，真实数据走职位表导入）。
func (s *MySQLStore) SeedPositions(positions []Position) error {
	var count int64
	if err := s.db.Model(&Position{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil // 已有数据，跳过
	}
	return s.db.Create(&positions).Error
}
