package runtime

import "context"

// userIDKey 用户身份的 context 键。
// 用户身份由 API 层（JWT）注入，经 Message.UserID 跨总线传播到节点，
// 工具执行时从 context 取回——模型无法伪造身份。
type userIDKey struct{}

// WithUserID 将用户 ID 注入 context。
func WithUserID(ctx context.Context, userID uint64) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

// UserIDFromContext 从 context 取用户 ID，未注入返回 0。
func UserIDFromContext(ctx context.Context) uint64 {
	if v, ok := ctx.Value(userIDKey{}).(uint64); ok {
		return v
	}
	return 0
}
