package tool

import (
	"context"
	"encoding/json"
	"errors"

	"ai-start/internal/types"
)

// ErrNoIdentity 缺少用户身份（工具写操作的安全红线：身份只能来自 JWT context）。
var ErrNoIdentity = errors.New("缺少用户身份，请重新登录")

// GetProfileFunc 取用户档案能力（由 logic 层注入）。
type GetProfileFunc func(ctx context.Context, userID uint64) (*types.UserProfile, error)

// GetProfileTool 用户档案查询工具。
type GetProfileTool struct {
	get GetProfileFunc
}

// NewGetProfileTool 创建档案查询工具。
func NewGetProfileTool(get GetProfileFunc) *GetProfileTool {
	return &GetProfileTool{get: get}
}

// Name 工具名。
func (t *GetProfileTool) Name() string { return "get_profile" }

// Description 功能描述。
func (t *GetProfileTool) Description() string {
	return "获取当前用户的报考档案（学历、专业、政治面貌、应届、目标地区、基层年限等）。用户身份自动从登录态获取，无需传参"
}

// ParamsJSON 参数 JSON Schema（无参数：身份来自登录态）。
func (t *GetProfileTool) ParamsJSON() string {
	return `{"type": "object", "properties": {}}`
}

// Execute 执行查询。
func (t *GetProfileTool) Execute(ctx context.Context, _ string) (string, error) {
	userID := UserIDFromContext(ctx)
	if userID == 0 {
		return "", ErrNoIdentity
	}
	profile, err := t.get(ctx, userID)
	if err != nil {
		return "", err
	}
	if profile == nil {
		return `{"message": "用户还没有档案，需要先完成条件解析"}`, nil
	}
	data, err := json.Marshal(profile)
	return string(data), err
}

// UpdateProfileFunc 档案更新能力（由 logic 层注入，内部走解析+合并规则）。
type UpdateProfileFunc func(ctx context.Context, userID uint64, text string) (*types.UserProfile, []string, error)

// UpdateProfileTool 档案更新工具：用户在对话中补充/修改条件时使用。
type UpdateProfileTool struct {
	update UpdateProfileFunc
}

// NewUpdateProfileTool 创建档案更新工具。
func NewUpdateProfileTool(update UpdateProfileFunc) *UpdateProfileTool {
	return &UpdateProfileTool{update: update}
}

// Name 工具名。
func (t *UpdateProfileTool) Name() string { return "update_profile" }

// Description 功能描述。
func (t *UpdateProfileTool) Description() string {
	return "更新当前用户的报考档案。当用户表达条件变更时使用（如「我入党了」「我工作两年了」）。传入用户原话即可"
}

// ParamsJSON 参数 JSON Schema。
func (t *UpdateProfileTool) ParamsJSON() string {
	return `{
		"type": "object",
		"properties": {
			"text": {"type": "string", "description": "用户表达变更的原话，如 我入党了"}
		},
		"required": ["text"]
	}`
}

// Execute 执行更新，返回变更摘要。
func (t *UpdateProfileTool) Execute(ctx context.Context, args string) (string, error) {
	userID := UserIDFromContext(ctx)
	if userID == 0 {
		return "", ErrNoIdentity
	}
	var params struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil || params.Text == "" {
		return "", errors.New("参数错误：text 必填")
	}
	profile, changed, err := t.update(ctx, userID, params.Text)
	if err != nil {
		return "", err
	}
	result, _ := json.Marshal(map[string]any{
		"changed_fields": changed,
		"profile":        profile,
	})
	return string(result), nil
}
