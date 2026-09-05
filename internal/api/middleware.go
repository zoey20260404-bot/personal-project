package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ai-start/internal/api/logic"
	"ai-start/internal/runtime"
)

// TraceMiddleware 链路追踪中间件：为每个请求生成 TraceID，
// 注入请求 context（贯穿多 Agent 消息）并回写响应头，便于跨 Agent 定位问题。
func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := newTraceID()
		c.Request = c.Request.WithContext(runtime.WithTraceID(c.Request.Context(), traceID))
		c.Header("X-Trace-Id", traceID)
		c.Next()
	}
}

// ctxUserIDKey 用户 ID 在 gin.Context 中的键。
const ctxUserIDKey = "user_id"

// AuthMiddleware JWT 鉴权中间件：解析 Authorization: Bearer <token>，
// 令牌校验委托业务层（UserService.ParseToken），注入 user_id。
func AuthMiddleware(users *logic.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		tokenString := strings.TrimPrefix(header, "Bearer ")
		if tokenString == header || tokenString == "" {
			Fail(c, http.StatusUnauthorized, "缺少或非法的 Authorization 头")
			c.Abort()
			return
		}
		userID, err := users.ParseToken(tokenString)
		if err != nil {
			Fail(c, http.StatusUnauthorized, err.Error())
			c.Abort()
			return
		}
		c.Set(ctxUserIDKey, userID)
		c.Next()
	}
}

// CurrentUserID 从请求上下文中取当前登录用户 ID（中间件已保证存在）。
func CurrentUserID(c *gin.Context) uint64 {
	if v, ok := c.Get(ctxUserIDKey); ok {
		if id, ok := v.(uint64); ok {
			return id
		}
	}
	return 0
}

// newTraceID 生成链路追踪 ID。
func newTraceID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "trace_" + hex.EncodeToString(b)
}
