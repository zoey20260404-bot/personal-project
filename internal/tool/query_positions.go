package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"ai-start/internal/store"
)

// QueryPositionsFunc 岗位查询能力（由 logic 层注入实现）。
type QueryPositionsFunc func(ctx context.Context, filter store.PositionFilter) ([]store.Position, int64, error)

// QueryPositionsTool 岗位查询工具：ReAct Agent 通过 function calling 调用。
// 实现由 logic 层注入（工具是薄壳，业务在业务层）。
type QueryPositionsTool struct {
	query QueryPositionsFunc
}

// NewQueryPositionsTool 创建岗位查询工具。
func NewQueryPositionsTool(query QueryPositionsFunc) *QueryPositionsTool {
	return &QueryPositionsTool{query: query}
}

// Name 工具名。
func (t *QueryPositionsTool) Name() string { return "query_positions" }

// Description 功能描述。
func (t *QueryPositionsTool) Description() string {
	return "查询公务员岗位库。可按省份、学历、专业大类、考试类型筛选，返回岗位列表（含进面分、报录比、备注）"
}

// ParamsJSON 参数 JSON Schema。
func (t *QueryPositionsTool) ParamsJSON() string {
	return `{
		"type": "object",
		"properties": {
			"province": {"type": "string", "description": "目标省份，如 广东；国考填 国家"},
			"major_category": {"type": "string", "description": "专业大类，如 计算机类"},
			"education": {"type": "string", "description": "学历：大专/本科/硕士/博士"},
			"exam_type": {"type": "string", "description": "国考 或 省考"},
			"keyword": {"type": "string", "description": "岗位名/单位关键词"}
		}
	}`
}

// Execute 执行查询。
func (t *QueryPositionsTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Province      string `json:"province"`
		MajorCategory string `json:"major_category"`
		Education     string `json:"education"`
		ExamType      string `json:"exam_type"`
		Keyword       string `json:"keyword"`
	}
	if args != "" {
		if err := json.Unmarshal([]byte(args), &params); err != nil {
			return "", fmt.Errorf("参数解析失败: %w", err)
		}
	}
	positions, total, err := t.query(ctx, store.PositionFilter{
		Province:      params.Province,
		MajorCategory: params.MajorCategory,
		Education:     params.Education,
		ExamType:      params.ExamType,
		Keyword:       params.Keyword,
		PageSize:      10, // 工具调用场景下取前 10 条，避免上下文过长
	})
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(map[string]any{"total": total, "positions": positions})
	if err != nil {
		return "", err
	}
	return string(result), nil
}
