package tool

import (
	"context"

	"ai-start/internal/runtime"
)

// 用户身份 helper 委托 runtime 包（同一 context 键，跨总线传播由 Message.UserID 承载）。

// WithUserID 将当前登录用户 ID 注入 context（API 层调用链起点设置）。
func WithUserID(ctx context.Context, userID uint64) context.Context {
	return runtime.WithUserID(ctx, userID)
}

// UserIDFromContext 从 context 取用户 ID，未注入返回 0（工具应拒绝执行写操作）。
func UserIDFromContext(ctx context.Context) uint64 {
	return runtime.UserIDFromContext(ctx)
}
