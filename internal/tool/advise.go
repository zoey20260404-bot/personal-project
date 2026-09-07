package tool

import (
	"context"
	"encoding/json"

	"ai-start/internal/store"
)

// RunAdviseFunc 选岗推荐能力（由 logic 层注入，内部走 advise 流程）。
type RunAdviseFunc func(ctx context.Context, userID uint64) (*store.Report, error)

// RunAdviseTool 选岗推荐触发工具：用户表达"帮我推荐岗位/生成报告"时调用。
type RunAdviseTool struct {
	run RunAdviseFunc
}

// NewRunAdviseTool 创建选岗推荐工具。
func NewRunAdviseTool(run RunAdviseFunc) *RunAdviseTool {
	return &RunAdviseTool{run: run}
}

// Name 工具名。
func (t *RunAdviseTool) Name() string { return "run_advise" }

// Description 功能描述。
func (t *RunAdviseTool) Description() string {
	return "生成专属选岗报告（智能匹配→竞争分析→冲稳保推荐）。用户明确要求推荐岗位/生成报告/给选岗建议时调用。耗时较长，期间会有进度提示"
}

// ParamsJSON 参数 JSON Schema（无参数：用户身份与档案自动获取）。
func (t *RunAdviseTool) ParamsJSON() string {
	return `{"type": "object", "properties": {}}`
}

// Execute 执行选岗推荐流程，返回报告 ID 与内容。
func (t *RunAdviseTool) Execute(ctx context.Context, _ string) (string, error) {
	userID := UserIDFromContext(ctx)
	if userID == 0 {
		return "", ErrNoIdentity
	}
	report, err := t.run(ctx, userID)
	if err != nil {
		return "", err
	}
	result, _ := json.Marshal(map[string]any{
		"report_id": report.ReportID,
		"report":    report.Content,
	})
	return string(result), nil
}
