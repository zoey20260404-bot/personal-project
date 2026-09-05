package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OK 统一成功响应：{"status": "...", "data": ...}（对齐 PRD 响应格式）。
func OK(c *gin.Context, status string, data any) {
	c.JSON(http.StatusOK, gin.H{"status": status, "data": data})
}

// Fail 统一错误响应：{"error": "..."}。
func Fail(c *gin.Context, httpCode int, msg string) {
	c.JSON(httpCode, gin.H{"error": msg})
}
