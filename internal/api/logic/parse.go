package logic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"ai-start/internal/agent"
	"ai-start/internal/store"
	"ai-start/internal/types"
)

// ParseService 条件解析域业务逻辑：
// 编排解析流程（经 Supervisor 走 Agent 总线）、维护用户权威档案。
//
// 档案规则（一个用户一份权威档案，存 user_profiles 表）：
//   - 首次解析存在未确认字段 → 返回 need_confirm，用户确认后才落档案
//   - 已有档案的后续解析：未确认/空缺的字段以历史档案值为准（不打扰用户），
//     确定的字段正常更新，直接返回 success
type ParseService struct {
	supervisor *agent.Supervisor // 调度中枢
	mysql      *store.MySQLStore // 用户存储，nil 时跳过持久化
	logger     *slog.Logger      // 日志器（注入）
	memory     *agent.Memory     // 长期记忆（档案变更沉淀为用户画像，feat003）
}

// NewParseService 创建条件解析服务。
func NewParseService(supervisor *agent.Supervisor, mysql *store.MySQLStore, logger *slog.Logger, memory *agent.Memory) *ParseService {
	return &ParseService{supervisor: supervisor, mysql: mysql, logger: logger, memory: memory}
}

// Parse 条件解析主流程：文本 + 图片（可同时进行）融合解析 → 结构化条件。
func (s *ParseService) Parse(ctx context.Context, userID uint64, content string, images []types.ImageInput, mode string) (*types.ParseResult, error) {
	result, err := s.supervisor.ParseInput(ctx, content, images)
	if err != nil {
		return nil, err
	}

	// 生成会话 ID；解析历史在确定最终状态后写入（见函数末尾）
	result.SessionID = newSessionID()
	profile := result.Profile
	if profile == nil {
		profile = result.ProfilePartial
	}

	// 首次解析：无历史档案
	existing := s.loadProfile(userID)
	if existing == nil {
		// 解析成功 → 直接落档案；need_confirm → 等用户确认（confirm 时落档案）
		if result.Status == types.StatusSuccess {
			s.upsertProfile(userID, profile)
		}
		s.saveUserSession(userID, result.SessionID, mode, sessionStatus(result.Status), profile)
		return result, nil
	}

	// 已有档案：未确认/空缺字段以历史值为准，直接成功（不再打扰用户确认）
	merged := mergeWithExisting(profile, result.UncertainFields, existing)
	s.upsertProfile(userID, merged)
	result.Status = types.StatusSuccess
	result.Profile = merged
	result.ProfilePartial = nil
	result.UncertainFields = nil
	result.Confidence = 1.0

	// 写入解析历史（最终状态：合并后为 completed）
	s.saveUserSession(userID, result.SessionID, mode, sessionStatus(result.Status), merged)
	return result, nil
}

// Confirm 确认修正：合并用户确认字段，生成完整条件并落档案（首次确认场景）。
func (s *ParseService) Confirm(userID uint64, sessionID string, partial *types.UserProfile, confirmedFields map[string]string) (*types.ParseResult, error) {
	profile := agent.Confirm(partial, confirmedFields)

	// 更新解析历史状态 + 落权威档案
	s.saveUserSession(userID, sessionID, "", "completed", profile)
	s.upsertProfile(userID, profile)

	return &types.ParseResult{
		Status:     types.StatusSuccess,
		Profile:    profile,
		Confidence: 1.0, // 人工确认后置信度视为 1
		SessionID:  sessionID,
		ParsedFrom: "confirmed",
	}, nil
}

// UpdateFromText 档案增量更新（chat 补充条件场景，feat002）：
// 用户口述变更 → Parser 解析 → 与历史档案合并（确定的覆盖、未提及的保留）→ 落档案。
// 返回更新后的档案与变更字段名列表。
func (s *ParseService) UpdateFromText(ctx context.Context, userID uint64, text string) (*types.UserProfile, []string, error) {
	result, err := s.supervisor.ParseInput(ctx, text, nil)
	if err != nil {
		return nil, nil, err
	}
	newProfile := result.Profile
	if newProfile == nil {
		newProfile = result.ProfilePartial
	}
	if newProfile == nil {
		return nil, nil, nil
	}

	existing := s.loadProfile(userID)
	if existing == nil {
		// 无历史档案：直接建档
		s.upsertProfile(userID, newProfile)
		return newProfile, []string{"(首次建档)"}, nil
	}

	merged := mergeWithExisting(newProfile, result.UncertainFields, existing)
	changed := diffProfiles(existing, merged)
	s.upsertProfile(userID, merged)
	// 档案变更沉淀为 user 作用域记忆（跨 Agent 共享的画像信号）
	if len(changed) > 0 && s.memory != nil {
		s.memory.Remember(ctx, store.MemoryRecord{
			UserID:  userID,
			Scope:   store.ScopeUser,
			Role:    "insight",
			Content: "用户档案更新：" + strings.Join(changed, "；"),
		})
	}
	return merged, changed, nil
}

// diffProfiles 对比两份档案，返回发生变化的字段名列表。
func diffProfiles(old, new *types.UserProfile) []string {
	var changed []string
	if old.Education != new.Education {
		changed = append(changed, "学历: "+old.Education+" → "+new.Education)
	}
	if old.Major != new.Major {
		changed = append(changed, "专业: "+old.Major+" → "+new.Major)
	}
	if old.PoliticalStatus != new.PoliticalStatus {
		changed = append(changed, "政治面貌: "+old.PoliticalStatus+" → "+new.PoliticalStatus)
	}
	if (old.IsFreshGraduate == nil) != (new.IsFreshGraduate == nil) ||
		(old.IsFreshGraduate != nil && new.IsFreshGraduate != nil && *old.IsFreshGraduate != *new.IsFreshGraduate) {
		changed = append(changed, "应届身份已更新")
	}
	if fmt.Sprintf("%v", old.TargetProvinces) != fmt.Sprintf("%v", new.TargetProvinces) {
		changed = append(changed, "目标省份已更新")
	}
	if old.WorkExperienceYears != new.WorkExperienceYears {
		changed = append(changed, fmt.Sprintf("基层年限: %d → %d", old.WorkExperienceYears, new.WorkExperienceYears))
	}
	return changed
}

// GetLatestProfile 获取用户权威档案（解析页直接展示）。
// 无档案时返回 nil, nil。
func (s *ParseService) GetLatestProfile(userID uint64) (*types.UserProfile, string, error) {
	rec := s.loadProfileRecord(userID)
	if rec == nil {
		return nil, "", nil
	}
	var profile types.UserProfile
	if err := json.Unmarshal([]byte(rec.Profile), &profile); err != nil {
		return nil, "", err
	}
	return &profile, "", nil
}

// mergeWithExisting 合并新解析结果与历史档案：
// 新结果中未确认（在 uncertain 列表）或空缺的字段，以历史档案值为准。
func mergeWithExisting(newProfile *types.UserProfile, uncertain []types.UncertainField, old *types.UserProfile) *types.UserProfile {
	if newProfile == nil {
		return old
	}
	uncertainSet := make(map[string]bool, len(uncertain))
	for _, uf := range uncertain {
		uncertainSet[uf.Field] = true
	}
	// keepOld：字段不确定或新值为空时保留历史值
	keepOld := func(field string, hasNewValue bool) bool {
		return uncertainSet[field] || !hasNewValue
	}

	merged := *newProfile // 拷贝
	if keepOld(types.FieldEducation, newProfile.Education != "") {
		merged.Education = old.Education
	}
	if keepOld(types.FieldMajor, newProfile.Major != "") {
		merged.Major = old.Major
	}
	if keepOld(types.FieldMajorCategory, newProfile.MajorCategory != "") {
		merged.MajorCategory = old.MajorCategory
	}
	if keepOld(types.FieldPoliticalStatus, newProfile.PoliticalStatus != "") {
		merged.PoliticalStatus = old.PoliticalStatus
	}
	if keepOld(types.FieldIsFreshGraduate, newProfile.IsFreshGraduate != nil) {
		merged.IsFreshGraduate = old.IsFreshGraduate
	}
	if keepOld(types.FieldTargetProvinces, len(newProfile.TargetProvinces) > 0) {
		merged.TargetProvinces = old.TargetProvinces
	}
	if newProfile.WorkExperienceYears == 0 {
		merged.WorkExperienceYears = old.WorkExperienceYears
	}
	if keepOld(types.FieldGender, newProfile.Gender != "") {
		merged.Gender = old.Gender
	}
	if keepOld(types.FieldAge, newProfile.Age != 0) {
		merged.Age = old.Age
	}
	if len(newProfile.OtherRequirements) == 0 {
		merged.OtherRequirements = old.OtherRequirements
	}
	return &merged
}

// loadProfileRecord 读取用户档案记录（不存在或存储不可用返回 nil）。
func (s *ParseService) loadProfileRecord(userID uint64) *store.UserProfileRecord {
	if s.mysql == nil {
		return nil
	}
	rec, err := s.mysql.GetUserProfile(userID)
	if err != nil {
		s.logger.Warn("读取用户档案失败", "err", err)
		return nil
	}
	return rec
}

// loadProfile 读取并反序列化用户档案。
func (s *ParseService) loadProfile(userID uint64) *types.UserProfile {
	rec := s.loadProfileRecord(userID)
	if rec == nil {
		return nil
	}
	var profile types.UserProfile
	if err := json.Unmarshal([]byte(rec.Profile), &profile); err != nil {
		s.logger.Warn("档案反序列化失败", "err", err)
		return nil
	}
	return &profile
}

// upsertProfile 保存/更新用户权威档案。
func (s *ParseService) upsertProfile(userID uint64, profile *types.UserProfile) {
	if s.mysql == nil || profile == nil {
		return
	}
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		s.logger.Warn("序列化档案失败（跳过）", "err", err)
		return
	}
	if err := s.mysql.UpsertUserProfile(userID, string(profileJSON)); err != nil {
		s.logger.Warn("保存用户档案失败（跳过）", "err", err)
	}
}

// saveUserSession 写入解析历史（user_sessions，仅留痕）。
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
