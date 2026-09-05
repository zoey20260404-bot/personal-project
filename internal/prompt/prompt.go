// Package prompt 提供 Prompt 模板库。
//
// 设计原则：Prompt 是代码资产，以 Go 常量集中定义（见 prompts_*.go），
// 一个 Agent 可拥有多个 Prompt 变体（如按模式/场景/流程阶段选用），
// 代码根据运行情况按名取用，不存在"Agent 绑定唯一 Prompt"。
// 支持 Go text/template 变量渲染（如 {{.Mode}}），便于按场景动态生成。
package prompt

import (
	"bytes"
	"fmt"
	"text/template"
)

// Store Prompt 模板库（并发安全：构建后只读）。
type Store struct {
	templates map[string]string // Prompt 名 → 模板原文
}

// NewStore 创建 Prompt 模板库，加载内置常量表（Defaults）。
func NewStore() *Store {
	return &Store{templates: Defaults}
}

// Get 按名取 Prompt 原文，不存在返回空串。
func (s *Store) Get(name string) string {
	return s.templates[name]
}

// Render 取 Prompt 并用模板变量渲染。
// vars 为模板变量（如 map[string]any{"Mode": "beginner"}）；模板无变量时原样返回。
func (s *Store) Render(name string, vars map[string]any) (string, error) {
	raw := s.Get(name)
	if raw == "" {
		return "", fmt.Errorf("prompt %q 不存在", name)
	}
	tpl, err := template.New(name).Parse(raw)
	if err != nil {
		return "", fmt.Errorf("prompt %q 模板解析失败: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("prompt %q 渲染失败: %w", name, err)
	}
	return buf.String(), nil
}

// Names 返回库中所有 Prompt 名。
func (s *Store) Names() []string {
	names := make([]string, 0, len(s.templates))
	for name := range s.templates {
		names = append(names, name)
	}
	return names
}
