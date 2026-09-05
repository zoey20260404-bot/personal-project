// Package tool 提供 Agent 可调用的工具注册表。
// 工具即函数：定义名称、描述、参数 Schema 与执行逻辑，
// ReAct Agent 通过 function calling 在推理循环中调用。
package tool

import (
	"context"
	"fmt"
	"sync"
)

// Tool Agent 可调用工具的接口。
type Tool interface {
	// Name 工具唯一名（function calling 中的 function name）。
	Name() string
	// Description 功能描述，模型据此决定何时调用。
	Description() string
	// ParamsJSON 参数的 JSON Schema 描述。
	ParamsJSON() string
	// Execute 执行工具，args 为模型给出的 JSON 参数串，返回文本结果。
	Execute(ctx context.Context, args string) (string, error)
}

// Registry 工具注册表（线程安全）。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry 创建空工具注册表。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 注册工具，重名会覆盖。
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get 按名获取工具，不存在返回 nil。
func (r *Registry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// Defs 按名字列表导出工具定义（供 LLM function calling 使用）。
func (r *Registry) Defs(names []string) []ToolDefView {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var defs []ToolDefView
	for _, name := range names {
		if t, ok := r.tools[name]; ok {
			defs = append(defs, ToolDefView{
				Name:        t.Name(),
				Description: t.Description(),
				ParamsJSON:  t.ParamsJSON(),
			})
		}
	}
	return defs
}

// ToolDefView 工具定义视图（避免 tool 包反向依赖 llm 包）。
type ToolDefView struct {
	Name        string
	Description string
	ParamsJSON  string
}

// Execute 按名执行工具；工具不存在时返回错误文本（供模型纠错，而非中断流程）。
func (r *Registry) Execute(ctx context.Context, name, args string) string {
	t := r.Get(name)
	if t == nil {
		return fmt.Sprintf(`{"error": "工具 %q 不存在"}`, name)
	}
	result, err := t.Execute(ctx, args)
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return result
}
