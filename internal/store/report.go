package store

import (
	"gorm.io/gorm"
)

// Report 选岗报告（PRD 5.4）。
type Report struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`             // 自增主键
	ReportID   string `gorm:"column:report_id;uniqueIndex;size:64"` // 报告唯一标识
	UserID     uint64 `gorm:"column:user_id;index"`                 // 归属用户
	SessionID  string `gorm:"column:session_id;size:64"`            // 会话标识
	Profile    string `gorm:"column:profile;type:json"`             // 用户条件快照
	Content    string `gorm:"column:content;type:longtext"`         // 报告正文（Markdown）
	ResultJSON string `gorm:"column:result_json;type:json"`         // 结构化策略（冲稳保分组）
	Status     string `gorm:"column:status;size:20;index"`          // processing / completed / failed
	CreatedAt  int64  `gorm:"autoCreateTime"`                       // 创建时间
	UpdatedAt  int64  `gorm:"autoUpdateTime"`                       // 更新时间
}

// TableName 指定表名。
func (Report) TableName() string { return "reports" }

// CreateReport 创建报告记录（result_json 为 JSON 列，空串非法，为空时跳过该字段）。
func (s *MySQLStore) CreateReport(r *Report) error {
	if r.ResultJSON == "" {
		return s.db.Omit("result_json").Create(r).Error
	}
	return s.db.Create(r).Error
}

// CompleteReport 更新报告内容与状态（生成完成/失败）。空字段不更新（result_json 为 JSON 列，空串非法）。
func (s *MySQLStore) CompleteReport(reportID, status, content, resultJSON string) error {
	updates := map[string]interface{}{"status": status, "content": content}
	if resultJSON != "" {
		updates["result_json"] = resultJSON
	}
	return s.db.Model(&Report{}).Where("report_id = ?", reportID).Updates(updates).Error
}

// GetReport 按报告 ID 查询（限定用户，防越权）。不存在返回 nil, nil。
func (s *MySQLStore) GetReport(reportID string, userID uint64) (*Report, error) {
	var report Report
	err := s.db.Where("report_id = ? AND user_id = ?", reportID, userID).First(&report).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// ListReports 查询用户报告列表（按创建时间倒序）。
func (s *MySQLStore) ListReports(userID uint64) ([]Report, error) {
	var reports []Report
	err := s.db.Select("id", "report_id", "user_id", "session_id", "status", "created_at", "updated_at").
		Where("user_id = ?", userID).Order("created_at DESC").Find(&reports).Error
	return reports, err
}
