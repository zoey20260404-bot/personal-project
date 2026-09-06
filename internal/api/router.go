package api

import (
	"github.com/gin-gonic/gin"
)

// NewRouter 构建 Gin 路由：只做路由注册与中间件挂载，不含任何业务逻辑。
func NewRouter(h *Handler) *gin.Engine {
	r := gin.Default()
	r.Use(TraceMiddleware())

	// 前端页面（单页应用，随需求迭代）
	r.StaticFile("/", "./web/index.html")

	// 健康检查（公开）
	r.GET("/health", func(c *gin.Context) {
		OK(c, "ok", nil)
	})

	// 认证接口（公开）
	auth := r.Group("/auth")
	{
		auth.POST("/register", h.Register) // 用户注册
		auth.POST("/login", h.Login)       // 用户登录，返回 JWT
	}

	// v1 业务接口分组（JWT 鉴权）
	v1 := r.Group("/api/v1", AuthMiddleware(h.svc.Services.User))
	{
		v1.POST("/parse", h.Parse)                // 条件解析（文字/图片）
		v1.POST("/parse/confirm", h.ParseConfirm) // 低置信度字段确认修正
		v1.GET("/profile", h.GetProfile)          // 用户条件档案（最近一次解析结果）
		v1.GET("/positions", h.ListPositions)     // 岗位查询（分页筛选，页面展示）
		v1.POST("/favorites", h.Favorite)         // 收藏/取消收藏岗位
		v1.GET("/favorites", h.ListFavorites)     // 收藏列表
		v1.POST("/chat", h.Chat)                  // 多轮追问（P1 占位）
		// TODO: POST /api/v1/advise 选岗推荐、GET /api/v1/reports/:id
	}

	return r
}
