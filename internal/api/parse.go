package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-start/internal/types"
)

// ParseRequest 条件解析请求体（PRD 3.2.1）。
// 支持多源融合：文本与图片可同时进行，后端融合出一份用户画像。
type ParseRequest struct {
	Content string             `json:"content"` // 文本内容（可选）
	Images  []types.ImageInput `json:"images"`  // 图片列表（可选，当前仅支持 diploma_image 毕业证/学位证）
	Mode    string             `json:"mode"`    // beginner / advanced
	// 用户身份从 JWT 获取（AuthMiddleware 注入），无需客户端传
}

// Parse 条件解析接口 POST /api/v1/parse。
// 文本+图片多源融合解析为结构化条件；低置信度时返回 need_confirm + profile_partial，
// 由客户端持有待确认数据（无状态设计，见 tech-design 4.2）。
func (h *Handler) Parse(c *gin.Context) {
	var req ParseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if req.Content == "" && len(req.Images) == 0 {
		Fail(c, http.StatusBadRequest, "文本与图片至少提供一项")
		return
	}

	result, err := h.svc.Services.Parse.Parse(c.Request.Context(),
		CurrentUserID(c), req.Content, req.Images, req.Mode)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, result.Status, result)
}

// GetProfile 用户档案接口 GET /api/v1/profile。
// 返回当前登录用户最近一次解析的条件快照，无记录时 data 为 null（前端展示空态）。
func (h *Handler) GetProfile(c *gin.Context) {
	profile, sessionID, err := h.svc.Services.Parse.GetLatestProfile(CurrentUserID(c))
	if err != nil {
		Fail(c, http.StatusInternalServerError, "查询用户档案失败: "+err.Error())
		return
	}
	OK(c, "success", gin.H{"profile": profile, "session_id": sessionID})
}

// ConfirmRequest 确认修正请求体。
// 无状态设计：客户端把 need_confirm 响应中的 profile_partial 原样带回，
// 附上人工确认的字段值即可，服务端不保存任何待确认状态。
type ConfirmRequest struct {
	SessionID       string            `json:"session_id" binding:"required"`       // 会话 ID（关联用户数据）
	ProfilePartial  types.UserProfile `json:"profile_partial" binding:"required"`  // need_confirm 返回的部分条件（原样带回）
	ConfirmedFields map[string]string `json:"confirmed_fields" binding:"required"` // 用户确认后的字段值
}

// ParseConfirm 确认修正接口 POST /api/v1/parse/confirm。
// 合并确认字段，生成完整条件并返回 success（PRD 3.2.2 响应格式）。
func (h *Handler) ParseConfirm(c *gin.Context) {
	var req ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}

	result, err := h.svc.Services.Parse.Confirm(CurrentUserID(c), req.SessionID, &req.ProfilePartial, req.ConfirmedFields)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	OK(c, types.StatusSuccess, result)
}
