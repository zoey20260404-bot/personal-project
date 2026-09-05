package tool

import (
	"context"
)

// QueryPositionsTool 岗位查询工具（MVP 占位实现，返回示例数据）。
// TODO: 接入 MySQL positions 表后改为真实检索（Researcher Agent 复用）。
type QueryPositionsTool struct{}

// Name 工具名。
func (t *QueryPositionsTool) Name() string { return "query_positions" }

// Description 功能描述。
func (t *QueryPositionsTool) Description() string {
	return "根据用户条件查询匹配的公务员岗位列表，返回岗位名称、单位、地区、条件要求等信息"
}

// ParamsJSON 参数 JSON Schema。
func (t *QueryPositionsTool) ParamsJSON() string {
	return `{
		"type": "object",
		"properties": {
			"province": {"type": "string", "description": "目标省份，如 广东；国考填 国家"},
			"major_category": {"type": "string", "description": "专业大类，如 计算机类"},
			"education": {"type": "string", "description": "学历：大专/本科/硕士/博士"}
		}
	}`
}

// Execute 执行查询（当前返回 Mock 数据）。
func (t *QueryPositionsTool) Execute(_ context.Context, _ string) (string, error) {
	return `[
		{"id": "pos_001", "name": "一级行政执法员", "department": "广州市税务局信息中心", "province": "广东", "city": "广州", "education_req": "本科及以上", "major_req": "计算机类"},
		{"id": "pos_002", "name": "综合管理岗", "department": "深圳市市场监督管理局", "province": "广东", "city": "深圳", "education_req": "本科", "major_req": "不限"}
	]`, nil
}
