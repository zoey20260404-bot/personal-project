package runtime

import (
	"context"
	"fmt"
)

// promptVarsKey Prompt 模板变量的 context 键。
// 动态 Prompt：每次调用可通过 context 注入变量（如 Mode/Profile），
// 经 Message.Vars 跨总线传播到节点。
type promptVarsKey struct{}

// WithPromptVars 将 Prompt 模板变量注入 context（由编排层在调用前设置）。
func WithPromptVars(ctx context.Context, vars map[string]any) context.Context {
	return context.WithValue(ctx, promptVarsKey{}, vars)
}

// PromptVarsFromContext 从 context 取 Prompt 模板变量，无则返回 nil。
func PromptVarsFromContext(ctx context.Context) map[string]any {
	if v, ok := ctx.Value(promptVarsKey{}).(map[string]any); ok {
		return v
	}
	return nil
}

// VarsToStrings 将变量表转为字符串映射（消息序列化用）。
func VarsToStrings(vars map[string]any) map[string]string {
	if len(vars) == 0 {
		return nil
	}
	out := make(map[string]string, len(vars))
	for k, v := range vars {
		out[k] = fmt.Sprint(v)
	}
	return out
}

// StringsToVars 字符串映射转回变量表。
func StringsToVars(vars map[string]string) map[string]any {
	if len(vars) == 0 {
		return nil
	}
	out := make(map[string]any, len(vars))
	for k, v := range vars {
		out[k] = v
	}
	return out
}
