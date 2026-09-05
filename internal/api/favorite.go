package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-start/internal/api/logic"
	"ai-start/internal/store"
)

// FavoriteRequest 收藏/取消收藏请求体（PRD 3.1 收藏岗位接口）。
// 用户身份从 JWT 获取，无需客户端传。
type FavoriteRequest struct {
	PositionID string `json:"position_id" binding:"required"` // 岗位 ID
	Action     string `json:"action" binding:"required"`      // add / remove
	ReportID   string `json:"report_id"`                      // 关联报告 ID（可选）
	Category   string `json:"category"`                       // rush / stable / safe / custom
	Notes      string `json:"notes"`                          // 用户备注
}

// Favorite 收藏接口 POST /api/v1/favorites（收藏/取消收藏）。
func (h *Handler) Favorite(c *gin.Context) {
	var req FavoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}

	userID := CurrentUserID(c)
	switch req.Action {
	case "add":
		created, err := h.svc.Services.Favorite.Add(userID, store.Favorite{
			PositionID: req.PositionID,
			ReportID:   req.ReportID,
			Category:   req.Category,
			Notes:      req.Notes,
		})
		if errors.Is(err, logic.ErrFavoriteStoreOffline) {
			Fail(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err != nil {
			Fail(c, http.StatusInternalServerError, "收藏失败: "+err.Error())
			return
		}
		if !created {
			// 重复收藏幂等返回
			OK(c, "success", gin.H{"message": "已收藏过该岗位"})
			return
		}
		OK(c, "success", nil)
	case "remove":
		err := h.svc.Services.Favorite.Remove(userID, req.PositionID)
		if errors.Is(err, logic.ErrFavoriteNotFound) {
			Fail(c, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, logic.ErrFavoriteStoreOffline) {
			Fail(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		if err != nil {
			Fail(c, http.StatusInternalServerError, "取消收藏失败: "+err.Error())
			return
		}
		OK(c, "success", nil)
	default:
		Fail(c, http.StatusBadRequest, logic.ErrInvalidAction.Error())
	}
}

// ListFavorites 收藏列表接口 GET /api/v1/favorites（当前登录用户）。
func (h *Handler) ListFavorites(c *gin.Context) {
	favorites, err := h.svc.Services.Favorite.List(CurrentUserID(c))
	if errors.Is(err, logic.ErrFavoriteStoreOffline) {
		Fail(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "查询收藏列表失败: "+err.Error())
		return
	}
	OK(c, "success", favorites)
}
