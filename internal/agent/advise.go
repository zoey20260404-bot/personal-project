package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"ai-start/internal/llm"
	"ai-start/internal/prompt"
	"ai-start/internal/runtime"
	"ai-start/internal/store"
	"ai-start/internal/types"
)

// adviseFlowInput advise 流程输入（researcher 的输入）。
type adviseFlowInput struct {
	Profile *types.UserProfile `json:"profile"` // 用户档案
	Mode    string             `json:"mode"`    // beginner / advanced
}

// adviseCandidate 候选岗位（精简字段，控制 LLM 上下文长度）。
type adviseCandidate struct {
	ID          uint64 `json:"id"`           // 岗位 ID
	PositionID  string `json:"position_id"`  // 业务 ID（pos_N，收藏用）
	Name        string `json:"name"`         // 岗位名称
	Department  string `json:"department"`   // 招录单位
	City        string `json:"city"`         // 工作地
	Education   string `json:"education"`    // 学历要求
	Major       string `json:"major"`        // 专业要求（大类/原文）
	Political   string `json:"political"`    // 政治面貌要求
	FreshOnly   bool   `json:"fresh_only"`   // 是否限应届
	WorkYears   int    `json:"work_years"`   // 基层年限要求
	ScoreLatest int    `json:"score_latest"` // 最近进面分
	ScoreYear   int    `json:"score_year"`   // 分数线年度
	Remarks     string `json:"remarks"`      // 备注（暗坑来源）
}

// NewResearcherNode 创建 researcher 节点（纯代码，不调 LLM）：
// 按用户档案六维硬条件检索候选岗位，候选不足时放宽专业/地域重试一次。
func NewResearcherNode(name string, mysql *store.MySQLStore, inboxSize int) *runtime.Node {
	return runtime.NewNode(name, inboxSize, func(ctx context.Context, msg runtime.Message) (runtime.Message, error) {
		runtime.EmitEvent(ctx, runtime.EventStatus, "📊 正在按你的条件检索岗位库…")

		var input adviseFlowInput
		if err := json.Unmarshal([]byte(msg.Content), &input); err != nil {
			return runtime.Message{}, fmt.Errorf("researcher 输入解析失败: %w", err)
		}
		if input.Profile == nil {
			return runtime.Message{}, fmt.Errorf("缺少用户档案")
		}
		if mysql == nil {
			return runtime.Message{}, fmt.Errorf("岗位库不可用")
		}

		// 六维硬条件检索
		candidates := queryCandidates(mysql, input.Profile, false)
		note := ""
		// 候选不足 → 放宽专业限制重试一次
		if len(candidates) < 4 {
			runtime.EmitEvent(ctx, runtime.EventStatus, "候选不足，正在放宽专业限制重试…")
			candidates = queryCandidates(mysql, input.Profile, true)
			note = "已放宽专业限制扩大候选范围"
		}

		out := adviseContext{Profile: input.Profile, Mode: input.Mode, Candidates: candidates, Note: note}
		data, _ := json.Marshal(out)
		runtime.EmitEvent(ctx, runtime.EventStatus, fmt.Sprintf("📊 检索到 %d 个候选岗位，开始竞争分析…", len(candidates)))
		return runtime.Message{Type: runtime.MsgTypeResult, Content: string(data)}, nil
	})
}

// queryCandidates 按档案检索候选岗位（relax=true 时放开专业大类限制）。
func queryCandidates(mysql *store.MySQLStore, profile *types.UserProfile, relaxMajor bool) []adviseCandidate {
	filter := store.PositionFilter{
		Education: profile.Education,
		Political: profile.PoliticalStatus,
		IsFresh:   profile.IsFreshGraduate,
		Provinces: profile.TargetProvinces,
		Page:      1,
		PageSize:  30,
		WorkYears: &profile.WorkExperienceYears,
	}
	if !relaxMajor {
		filter.MajorCategory = profile.MajorCategory
	}
	positions, _, err := mysql.QueryPositions(filter)
	if err != nil {
		return nil
	}
	candidates := make([]adviseCandidate, 0, len(positions))
	for _, p := range positions {
		major := p.MajorReqCategory
		if major == "" {
			major = p.MajorReqExact
		}
		candidates = append(candidates, adviseCandidate{
			ID:          p.ID,
			PositionID:  fmt.Sprintf("pos_%d", p.ID),
			Name:        p.PositionName,
			Department:  p.Department,
			City:        p.City,
			Education:   p.EducationReq,
			Major:       major,
			Political:   p.PoliticalReq,
			FreshOnly:   p.FreshGraduateReq != nil && *p.FreshGraduateReq,
			WorkYears:   p.WorkExperienceYearsReq,
			ScoreLatest: p.ScoreLatest,
			ScoreYear:   p.ScoreYear,
			Remarks:     p.Remarks,
		})
	}
	return candidates
}

// NewLLMStepNode 创建单步 LLM 节点（analyzer/strategist/responder）：
// 渲染 Prompt（变量含 Input=上游输出、Mode=用户模式）→ 单次调用 → 返回内容。
// 非 ReAct 循环——流水线节点的行为是确定的，不需要模型自主选工具。
func NewLLMStepNode(name, label string, cfg AgentConfigLite, manager *llm.Manager, prompts *prompt.Store, inboxSize int) *runtime.Node {
	return runtime.NewNode(name, inboxSize, func(ctx context.Context, msg runtime.Message) (runtime.Message, error) {
		runtime.EmitEvent(ctx, runtime.EventStatus, label)

		vars := runtime.PromptVarsFromContext(ctx)
		if vars == nil {
			vars = map[string]any{}
		}
		vars["Input"] = msg.Content

		system, err := prompts.Render(cfg.Prompt, vars)
		if err != nil {
			system = prompts.Get(cfg.Prompt) // 渲染失败回退原文
		}

		resp, err := manager.ChatCompletion(ctx, cfg.Model,
			[]llm.Message{{Role: "user", Content: msg.Content}},
			&llm.Options{System: system, Temperature: cfg.Temperature})
		if err != nil {
			return runtime.Message{}, fmt.Errorf("%s 模型调用失败: %w", name, err)
		}
		// 输出校验兜底（LLM 输出不可信，规则层修正）
		content := resp.Content
		if cfg.Validator != nil {
			validated, verr := cfg.Validator(msg.Content, content)
			if verr != nil {
				return runtime.Message{}, fmt.Errorf("%s 输出校验失败: %w", name, verr)
			}
			content = validated
		}
		return runtime.Message{Type: runtime.MsgTypeResult, Content: content}, nil
	})
}

// adviseContext advise 流水线各节点间传递的上下文包裹（数据在节点间不丢失）。
type adviseContext struct {
	Profile    *types.UserProfile `json:"profile"`            // 用户档案
	Mode       string             `json:"mode"`               // 用户模式
	Candidates []adviseCandidate  `json:"candidates"`         // 候选岗位（全程携带，供报告引用真实名称）
	Analysis   json.RawMessage    `json:"analysis,omitempty"` // analyzer 输出
	Strategy   json.RawMessage    `json:"strategy,omitempty"` // strategist 输出
	Note       string             `json:"note,omitempty"`     // 候选不足说明
}

// adviseStepSpec advise 各 LLM 节点的行为定义。
type adviseStepSpec struct {
	// LLMInput 从上下文提取发给模型的内容。
	LLMInput func(ac *adviseContext) string
	// Wrap 把模型输出并入上下文（携带 candidates 传给下游）。
	Wrap func(ac *adviseContext, output string) *adviseContext
	// Validator 输出校验（可选）。
	Validator func(input, output string) (string, error)
}

// NewAdviseStepNode 创建 advise 流水线节点：按 adviseContext 包裹收发，数据不丢失。
func NewAdviseStepNode(name, label string, cfg AgentConfigLite, spec adviseStepSpec, manager *llm.Manager, prompts *prompt.Store, inboxSize int) *runtime.Node {
	return runtime.NewNode(name, inboxSize, func(ctx context.Context, msg runtime.Message) (runtime.Message, error) {
		runtime.EmitEvent(ctx, runtime.EventStatus, label)

		var ac adviseContext
		if err := json.Unmarshal([]byte(msg.Content), &ac); err != nil {
			return runtime.Message{}, fmt.Errorf("%s 输入解析失败: %w", name, err)
		}

		vars := runtime.PromptVarsFromContext(ctx)
		if vars == nil {
			vars = map[string]any{}
		}
		vars["Input"] = spec.LLMInput(&ac)
		if ac.Mode != "" {
			vars["Mode"] = ac.Mode
		}

		system, err := prompts.Render(cfg.Prompt, vars)
		if err != nil {
			system = prompts.Get(cfg.Prompt)
		}

		resp, err := manager.ChatCompletion(ctx, cfg.Model,
			[]llm.Message{{Role: "user", Content: spec.LLMInput(&ac)}},
			&llm.Options{System: system, Temperature: cfg.Temperature})
		if err != nil {
			return runtime.Message{}, fmt.Errorf("%s 模型调用失败: %w", name, err)
		}

		// 输出校验兜底（LLM 输出不可信，规则修正）
		content := resp.Content
		if spec.Validator != nil {
			inJSON, _ := json.Marshal(ac)
			validated, verr := spec.Validator(string(inJSON), content)
			if verr != nil {
				return runtime.Message{}, fmt.Errorf("%s 输出校验失败: %w", name, verr)
			}
			content = validated
		}

		// Wrap 为 nil 时直接输出原文（如 responder 的最终报告）
		if spec.Wrap == nil {
			return runtime.Message{Type: runtime.MsgTypeResult, Content: content}, nil
		}
		out, _ := json.Marshal(spec.Wrap(&ac, content))
		return runtime.Message{Type: runtime.MsgTypeResult, Content: string(out)}, nil
	})
}

// validateAnalyses 校验 analyzer 输出：剔除不存在的 position_id（LLM 幻觉兜底）。
// input 为 adviseContext JSON（含 candidates），output 为 analyzer 的分析 JSON。
func validateAnalyses(input, output string) (string, error) {
	// 提取候选岗位 ID 集合
	var ac adviseContext
	if err := json.Unmarshal([]byte(extractJSON(input)), &ac); err != nil {
		return "", fmt.Errorf("上下文解析失败: %w", err)
	}
	validIDs := make(map[string]bool, len(ac.Candidates))
	for _, c := range ac.Candidates {
		validIDs[c.PositionID] = true
	}

	// 解析分析结果（容错 markdown 代码块）
	var result struct {
		Analyses []struct {
			PositionID string `json:"position_id"`
		} `json:"analyses"`
	}
	raw := extractJSON(output)
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return "", fmt.Errorf("分析结果解析失败: %w", err)
	}
	var filtered []map[string]any
	// 重新按原始 JSON 过滤（保留完整字段）
	var full struct {
		Analyses []json.RawMessage `json:"analyses"`
	}
	if err := json.Unmarshal([]byte(raw), &full); err != nil {
		return "", err
	}
	for _, a := range full.Analyses {
		var item map[string]any
		if err := json.Unmarshal(a, &item); err != nil {
			continue
		}
		if id, ok := item["position_id"].(string); ok && validIDs[id] {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		return "", fmt.Errorf("分析结果中没有有效岗位（全部为模型编造 ID）")
	}
	out, err := json.Marshal(map[string]any{"analyses": filtered})
	return string(out), err
}

type AgentConfigLite struct {
	Model       string
	Prompt      string
	Temperature *float64
	// Validator 输出校验钩子（可选）：输入为（上游内容， 模型输出），返回修正后的输出。
	// 用于 LLM 输出不可信场景的规则兜底（如 analyzer 的 position_id 真实性校验）。
	Validator func(input, output string) (string, error)
}
