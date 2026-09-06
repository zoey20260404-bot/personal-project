package agent

import "context"

// promptVarsKey Prompt 模板变量的 context 键。
// 动态 Prompt：每次调用可通过 context 注入不同变量（如 Mode/Profile/Memories），
// Agent 运行时渲染对应变体，而不是固定 Prompt。
type promptVarsKey struct{}

// WithPromptVars 将 Prompt 模板变量注入 context（由编排层在调用前设置）。
func WithPromptVars(ctx context.Context, vars map[string]any) context.Context {
	return context.WithValue(ctx, promptVarsKey{}, vars)
}

// promptVarsFromContext 从 context 取 Prompt 模板变量，无则返回 nil。
func promptVarsFromContext(ctx context.Context) map[string]any {
	if v, ok := ctx.Value(promptVarsKey{}).(map[string]any); ok {
		return v
	}
	return nil
}
