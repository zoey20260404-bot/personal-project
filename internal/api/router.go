package api

import (
	"github.com/gin-gonic/gin"
)

// NewRouter 构建 Gin 路由，集中注册所有 HTTP 接口。
func NewRouter() *gin.Engine {
	r := gin.Default()

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// v1 版本接口分组
	v1 := r.Group("/api/v1")
	{
		v1.POST("/chat", ChatHandler) // 对话接口
	}

	return r
}
