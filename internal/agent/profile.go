// Package agent 是多 Agent 协作的核心编排层（v1 手写实现，不依赖外部 Agent 框架）。
// 包含：Supervisor 调度中枢、Parser Agent（条件解析）、Memory（长期记忆），
// 后续迭代补充 Researcher / Analyzer / Strategist / Responder Agent。
package agent

import "ai-start/internal/types"

// 数据模型统一定义在 internal/model（避免 agent 与 store 循环依赖），
// 此处用类型别名暴露，保持 agent 包内引用方式不变。

type (
	// UserProfile 标准化用户条件结构（详见 types.UserProfile）。
	UserProfile = types.UserProfile
	// UncertainField OCR/解析低置信度字段（详见 types.UncertainField）。
	UncertainField = types.UncertainField
	// ParseResult 条件解析结果（详见 types.ParseResult）。
	ParseResult = types.ParseResult
)

// 解析状态常量（转发 model 包定义）。
const (
	StatusSuccess     = types.StatusSuccess
	StatusNeedConfirm = types.StatusNeedConfirm
)

// confidenceThreshold 字段置信度阈值（PRD 2.1：任一字段 < 0.85 即进入确认流程）。
const confidenceThreshold = 0.85
