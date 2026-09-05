// Package api 提供对外 HTTP 接口层（Gin 框架）。
// 职责仅限 HTTP 协议：路由注册、参数解析/校验、调用业务层、响应封装。
// 业务逻辑一律下沉到 internal/service，本层不直接触碰存储。
package api

import (
	"ai-start/internal/svc"
)

// Handler HTTP 接口处理器：仅持有服务上下文，经 svc.Services 调用业务层。
type Handler struct {
	svc *svc.ServiceContext
}

// NewHandler 创建 Handler 实例。
func NewHandler(s *svc.ServiceContext) *Handler {
	return &Handler{svc: s}
}
