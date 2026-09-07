package store

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrUserExists 用户名已存在。
var ErrUserExists = errors.New("用户名已存在")

// User 用户表：正式用户体系（userId + 密码），替代 MVP 早期的设备 ID 方案。
type User struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`            // 用户 ID
	Username     string `gorm:"column:username;uniqueIndex;size:64"` // 登录用户名（唯一）
	PasswordHash string `gorm:"column:password_hash;size:128"`       // bcrypt 密码哈希
	CreatedAt    int64  `gorm:"autoCreateTime"`                      // 创建时间
	UpdatedAt    int64  `gorm:"autoUpdateTime"`                      // 更新时间
}

// TableName 指定表名。
func (User) TableName() string { return "users" }

// UserProfileRecord 用户权威档案（一用户一条）：解析确认后的条件画像。
// 与 user_sessions（每次解析的历史记录）区分：档案是唯一权威当前值。
type UserProfileRecord struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`   // 自增主键
	UserID    uint64 `gorm:"column:user_id;uniqueIndex"` // 归属用户（唯一）
	Profile   string `gorm:"column:profile;type:json"`   // 条件画像（UserProfile JSON）
	CreatedAt int64  `gorm:"autoCreateTime"`             // 创建时间
	UpdatedAt int64  `gorm:"autoUpdateTime"`             // 更新时间
}

// TableName 指定表名。
func (UserProfileRecord) TableName() string { return "user_profiles" }

// UserSession 用户查询会话（PRD 5.3），归属注册用户。
type UserSession struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`              // 自增主键
	SessionID string `gorm:"column:session_id;uniqueIndex;size:64"` // 会话唯一标识
	UserID    uint64 `gorm:"column:user_id;index"`                  // 归属用户 ID
	Mode      string `gorm:"column:mode;size:20"`                   // beginner | advanced
	Status    string `gorm:"column:status;size:20"`                 // parsing | confirming | completed
	Profile   string `gorm:"column:profile;type:json"`              // 用户条件快照（UserProfile JSON）
	CreatedAt int64  `gorm:"autoCreateTime"`                        // 创建时间
	UpdatedAt int64  `gorm:"autoUpdateTime"`                        // 更新时间
}

// TableName 指定表名。
func (UserSession) TableName() string { return "user_sessions" }

// Favorite 用户收藏（PRD 5.5）。
// user_id + position_id 联合唯一，防止重复收藏。
type Favorite struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`                                 // 自增主键
	UserID     uint64 `gorm:"column:user_id;uniqueIndex:idx_user_position"`             // 归属用户 ID
	PositionID string `gorm:"column:position_id;uniqueIndex:idx_user_position;size:64"` // 岗位 ID
	ReportID   string `gorm:"column:report_id;size:64"`                                 // 关联报告 ID
	Category   string `gorm:"column:category;size:20"`                                  // rush | stable | safe | custom
	Notes      string `gorm:"column:notes;type:text"`                                   // 用户备注
	CreatedAt  int64  `gorm:"autoCreateTime"`                                           // 创建时间
}

// TableName 指定表名。
func (Favorite) TableName() string { return "favorites" }

// MySQLStore MySQL 存储客户端（用户、会话、收藏等结构化业务数据）。
type MySQLStore struct {
	db *gorm.DB
}

// NewMySQLStore 建立 MySQL 连接并自动迁移表结构。
// MVP 阶段用 AutoMigrate 建表，生产环境应使用迁移工具管理 DDL。
func NewMySQLStore(dsn string) (*MySQLStore, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	if err := db.AutoMigrate(&User{}, &UserSession{}, &UserProfileRecord{}, &Favorite{}, &Position{}, &Report{}); err != nil {
		return nil, fmt.Errorf("迁移表结构失败: %w", err)
	}
	return &MySQLStore{db: db}, nil
}

// CreateUser 创建用户；用户名重复返回 ErrUserExists。
func (s *MySQLStore) CreateUser(user *User) error {
	err := s.db.Create(user).Error
	if err != nil && strings.Contains(err.Error(), "Duplicate entry") {
		return ErrUserExists
	}
	return err
}

// GetUserByUsername 按用户名查询用户，不存在返回 nil, nil。
func (s *MySQLStore) GetUserByUsername(username string) (*User, error) {
	var user User
	err := s.db.Where("username = ?", username).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpsertSession 保存/更新用户会话（按 session_id 存在则更新，否则新建）。
// 用于解析成功、确认完成后持久化用户条件快照。
// 更新时仅覆盖非空/非零字段，避免确认流程中把已写入的 mode 等字段清空。
func (s *MySQLStore) UpsertSession(session *UserSession) error {
	var existing UserSession
	err := s.db.Where("session_id = ?", session.SessionID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return s.db.Create(session).Error
	}
	if err != nil {
		return fmt.Errorf("查询用户会话失败: %w", err)
	}
	// 更新已有会话：状态与条件快照必更新，其余字段仅在有值时覆盖
	updates := map[string]interface{}{
		"status":  session.Status,
		"profile": session.Profile,
	}
	if session.Mode != "" {
		updates["mode"] = session.Mode
	}
	if session.UserID != 0 {
		updates["user_id"] = session.UserID
	}
	return s.db.Model(&existing).Updates(updates).Error
}

// GetUserProfile 查询用户权威档案，不存在返回 nil, nil。
func (s *MySQLStore) GetUserProfile(userID uint64) (*UserProfileRecord, error) {
	var rec UserProfileRecord
	err := s.db.Where("user_id = ?", userID).First(&rec).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpsertUserProfile 保存/更新用户权威档案（按 user_id 唯一）。
func (s *MySQLStore) UpsertUserProfile(userID uint64, profileJSON string) error {
	rec := UserProfileRecord{UserID: userID, Profile: profileJSON}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"profile"}),
	}).Create(&rec).Error
}

// GetLatestSessionByUser 查询用户最近的会话记录（按创建时间倒序取第一条）。
// 不存在返回 nil, nil。
func (s *MySQLStore) GetLatestSessionByUser(userID uint64) (*UserSession, error) {
	var session UserSession
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").First(&session).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// AddFavorite 收藏岗位；重复收藏（同用户同岗位）不报错，created 返回 false。
func (s *MySQLStore) AddFavorite(fav *Favorite) (created bool, err error) {
	result := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(fav)
	return result.RowsAffected > 0, result.Error
}

// RemoveFavorite 取消收藏（按用户 ID + 岗位 ID 删除）。
func (s *MySQLStore) RemoveFavorite(userID uint64, positionID string) (int64, error) {
	result := s.db.Where("user_id = ? AND position_id = ?", userID, positionID).Delete(&Favorite{})
	return result.RowsAffected, result.Error
}

// ListFavorites 查询用户收藏列表（按创建时间倒序）。
func (s *MySQLStore) ListFavorites(userID uint64) ([]Favorite, error) {
	var favorites []Favorite
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&favorites).Error
	return favorites, err
}
