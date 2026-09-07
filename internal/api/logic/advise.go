package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"ai-start/internal/runtime"
	"ai-start/internal/store"
)

// AdviseService 选岗推荐业务逻辑（feat004 冲稳保主链路）。
type AdviseService struct {
	flows  *runtime.FlowExecutor // 流程编排器（advise 流程）
	mysql  *store.MySQLStore     // 报告持久化
	parse  *ParseService         // 档案服务
	logger *slog.Logger          // 日志器（注入）
}

// NewAdviseService 创建选岗推荐服务。
func NewAdviseService(flows *runtime.FlowExecutor, mysql *store.MySQLStore, parse *ParseService, logger *slog.Logger) *AdviseService {
	return &AdviseService{flows: flows, mysql: mysql, parse: parse, logger: logger}
}

// ErrNoProfile 用户尚未建档。
var ErrNoProfile = errors.New("还没有你的报考档案，请先在「条件解析」页完成条件输入")

// Run 执行选岗推荐流程：档案 → 四节点流水线 → 报告落库。
// 调用方的 ctx 携带 sink（SSE 进度事件经流程节点透传）。
func (s *AdviseService) Run(ctx context.Context, userID uint64, sessionID, mode string) (*store.Report, error) {
	// 1. 取档案
	profile, _, err := s.parse.GetLatestProfile(userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrNoProfile
	}

	// 2. 建档（processing），失败标记 failed
	reportID := "rep_" + newSessionID()[5:]
	report := &store.Report{
		ReportID:  reportID,
		UserID:    userID,
		SessionID: sessionID,
		Status:    "processing",
	}
	if profileJSON, err := json.Marshal(profile); err == nil && s.mysql != nil {
		report.Profile = string(profileJSON)
		if err := s.mysql.CreateReport(report); err != nil {
			s.logger.Warn("报告建档失败", "err", err, "report_id", reportID)
		} else {
			s.logger.Info("报告建档", "report_id", reportID)
		}
	}

	// 3. 执行 advise 流程
	input, _ := json.Marshal(map[string]any{"profile": profile, "mode": mode})
	content, err := s.flows.Run(ctx, "advise", string(input))
	if err != nil {
		s.logger.Warn("advise 流程失败", "err", err, "user_id", userID)
		if s.mysql != nil {
			_ = s.mysql.CompleteReport(reportID, "failed", "", err.Error())
		}
		return nil, fmt.Errorf("报告生成失败: %w", err)
	}

	// 4. 落库（completed）
	if s.mysql != nil {
		if err := s.mysql.CompleteReport(reportID, "completed", content, ""); err != nil {
			s.logger.Warn("报告落库失败", "err", err, "report_id", reportID)
		}
	}
	report.Status = "completed"
	report.Content = content
	return report, nil
}

// ListReports 用户报告列表。
func (s *AdviseService) ListReports(userID uint64) ([]store.Report, error) {
	if s.mysql == nil {
		return nil, nil
	}
	return s.mysql.ListReports(userID)
}

// GetReport 报告详情（限定当前用户）。
func (s *AdviseService) GetReport(reportID string, userID uint64) (*store.Report, error) {
	if s.mysql == nil {
		return nil, errors.New("存储不可用")
	}
	return s.mysql.GetReport(reportID, userID)
}
