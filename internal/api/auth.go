package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-start/internal/api/logic"
	"ai-start/internal/store"
)

// authRequest 注册/登录请求体。
type authRequest struct {
	Username string `json:"username" binding:"required"` // 用户名
	Password string `json:"password" binding:"required"` // 密码（传输层应走 HTTPS）
}

// Register 用户注册 POST /auth/register。
func (h *Handler) Register(c *gin.Context) {
	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "用户名和密码必填")
		return
	}

	userID, err := h.svc.Services.User.Register(req.Username, req.Password)
	switch {
	case errors.Is(err, logic.ErrPasswordTooShort):
		Fail(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, logic.ErrUserStoreOffline):
		Fail(c, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, store.ErrUserExists):
		Fail(c, http.StatusConflict, "用户名已存在")
	case err != nil:
		Fail(c, http.StatusInternalServerError, "注册失败: "+err.Error())
	default:
		OK(c, "success", gin.H{"user_id": userID})
	}
}

// Login 用户登录 POST /auth/login，成功返回 JWT。
func (h *Handler) Login(c *gin.Context) {
	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "用户名和密码必填")
		return
	}

	token, userID, err := h.svc.Services.User.Login(req.Username, req.Password)
	switch {
	case errors.Is(err, logic.ErrInvalidCredentials):
		Fail(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, logic.ErrUserStoreOffline):
		Fail(c, http.StatusServiceUnavailable, err.Error())
	case err != nil:
		Fail(c, http.StatusInternalServerError, "登录失败: "+err.Error())
	default:
		OK(c, "success", gin.H{"token": token, "user_id": userID})
	}
}
