package logic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"

	"ai-start/internal/agent"
	"ai-start/internal/store"
	"ai-start/internal/types"
)

// ParseService 条件解析域业务逻辑：
// 编排解析流程（经 Supervisor 走 Agent 总线）、生成会话 ID、持久化用户条件快照。
type ParseService struct {
	supervisor *agent.Supervisor // 调度中枢
	mysql      *store.MySQLStore // 用户会话存储，nil 时跳过持久化
	logger     *slog.Logger      // 日志器（注入）
}

// NewParseService 创建条件解析服务。
func NewParseService(supervisor *agent.Supervisor, mysql *store.MySQLStore, logger *slog.Logger) *ParseService {
	return &ParseService{supervisor: supervisor, mysql: mysql, logger: logger}
}

// Parse 条件解析主流程：文本 + 图片（可同时进行）融合解析 → 结构化条件。
// need_confirm 的待确认数据由客户端持有（无状态设计，见 tech-design 4.2）。
func (s *ParseService) Parse(ctx context.Context, userID uint64, content string, images []types.ImageInput, mode string) (*types.ParseResult, error) {
	result, err := s.supervisor.ParseInput(ctx, content, images)
	if err != nil {
		return nil, err
	}

	// 生成会话 ID（用于关联用户数据与后续追问）
	result.SessionID = newSessionID()

	// 用户条件快照持久化到 MySQL（归属当前登录用户）
	profile := result.Profile
	if profile == nil {
		profile = result.ProfilePartial
	}
	s.saveUserSession(userID, result.SessionID, mode, sessionStatus(result.Status), profile)

	return result, nil
}

// Confirm 确认修正：合并用户确认字段，生成完整条件并更新快照。
func (s *ParseService) Confirm(userID uint64, sessionID string, partial *types.UserProfile, confirmedFields map[string]string) (*types.ParseResult, error) {
	profile := agent.Confirm(partial, confirmedFields)

	// 更新 MySQL 中的用户条件快照，状态流转为 completed
	s.saveUserSession(userID, sessionID, "", "completed", profile)

	return &types.ParseResult{
		Status:     types.StatusSuccess,
		Profile:    profile,
		Confidence: 1.0, // 人工确认后置信度视为 1
		SessionID:  sessionID,
		ParsedFrom: "confirmed",
	}, nil
}

// GetLatestProfile 获取用户最近一次的条件快照（用于解析页直接展示已有档案）。
// 无记录时返回 nil, nil。
func (s *ParseService) GetLatestProfile(userID uint64) (*types.UserProfile, string, error) {
	if s.mysql == nil {
		return nil, "", nil
	}
	session, err := s.mysql.GetLatestSessionByUser(userID)
	if err != nil || session == nil {
		return nil, "", err
	}
	var profile types.UserProfile
	if err := json.Unmarshal([]byte(session.Profile), &profile); err != nil {
		return nil, "", err
	}
	return &profile, session.SessionID, nil
}

// saveUserSession 将用户条件快照持久化到 MySQL（MySQL 不可用时静默跳过）。
func (s *ParseService) saveUserSession(userID uint64, sessionID, mode, status string, profile *types.UserProfile) {
	if s.mysql == nil || sessionID == "" || profile == nil {
		return
	}
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		s.logger.Warn("序列化用户条件失败（跳过持久化）", "err", err)
		return
	}
	if err := s.mysql.UpsertSession(&store.UserSession{
		SessionID: sessionID,
		UserID:    userID,
		Mode:      mode,
		Status:    status,
		Profile:   string(profileJSON),
	}); err != nil {
		s.logger.Warn("保存用户会话失败（跳过）", "err", err)
	}
}

// sessionStatus 将解析状态映射为会话表状态。
func sessionStatus(parseStatus string) string {
	if parseStatus == types.StatusNeedConfirm {
		return "confirming"
	}
	return "completed"
}

// newSessionID 生成会话 ID，格式：sess_ + 8位随机十六进制。
func newSessionID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "sess_" + hex.EncodeToString(b)
}
