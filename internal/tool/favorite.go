package tool

import (
	"context"
	"encoding/json"
	"errors"
)

// AddFavoriteFunc 收藏能力（由 logic 层注入）。
type AddFavoriteFunc func(ctx context.Context, userID uint64, positionID string) error

// AddFavoriteTool 收藏岗位工具。
type AddFavoriteTool struct {
	add AddFavoriteFunc
}

// NewAddFavoriteTool 创建收藏工具。
func NewAddFavoriteTool(add AddFavoriteFunc) *AddFavoriteTool {
	return &AddFavoriteTool{add: add}
}

// Name 工具名。
func (t *AddFavoriteTool) Name() string { return "add_favorite" }

// Description 功能描述。
func (t *AddFavoriteTool) Description() string {
	return "收藏指定岗位。仅在用户明确表示收藏/想要某个岗位时调用"
}

// ParamsJSON 参数 JSON Schema。
func (t *AddFavoriteTool) ParamsJSON() string {
	return `{
		"type": "object",
		"properties": {
			"position_id": {"type": "string", "description": "岗位 ID，如 pos_3"}
		},
		"required": ["position_id"]
	}`
}

// Execute 执行收藏。
func (t *AddFavoriteTool) Execute(ctx context.Context, args string) (string, error) {
	userID := UserIDFromContext(ctx)
	if userID == 0 {
		return "", ErrNoIdentity
	}
	var params struct {
		PositionID string `json:"position_id"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil || params.PositionID == "" {
		return "", errors.New("参数错误：position_id 必填")
	}
	if err := t.add(ctx, userID, params.PositionID); err != nil {
		return "", err
	}
	return `{"message": "收藏成功"}`, nil
}
