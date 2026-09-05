package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"ai-start/internal/llm"
	"ai-start/internal/prompt"
	"ai-start/internal/types"

	"golang.org/x/sync/errgroup"
)

// ParserAgent 条件解析引擎（PRD 2.1 模块1）。
// 将文字/图片输入转换为标准化 UserProfile，模型渠道不可用时降级为关键词解析。
// Prompt 不固定：按 promptName 从 Prompt 库取用（prompt.Defaults 中的常量），
// 可按场景选用不同变体。
type ParserAgent struct {
	llm        *llm.Manager  // 模型管理器
	channel    string        // 文本渠道名（agents.parser.model 配置）
	prompts    *prompt.Store // Prompt 模板库
	promptName string        // 默认 Prompt 名（agents.parser.prompt 配置）
}

// NewParserAgent 创建 Parser Agent。channel 为空时使用默认 chat 渠道。
func NewParserAgent(manager *llm.Manager, channel string, prompts *prompt.Store, promptName string) *ParserAgent {
	if channel == "" {
		channel = llm.ChannelChat
	}
	if promptName == "" {
		promptName = prompt.NameParser
	}
	return &ParserAgent{llm: manager, channel: channel, prompts: prompts, promptName: promptName}
}

// Name 实现 Agent 标识。
func (p *ParserAgent) Name() string { return "parser" }

// systemPrompt 取当前生效的系统提示词（按名从 Prompt 库取用）。
func (p *ParserAgent) systemPrompt() string {
	if p.prompts != nil {
		if s := p.prompts.Get(p.promptName); s != "" {
			return s
		}
	}
	return prompt.Defaults[p.promptName]
}

// ParseMulti 多源融合解析：文本 + 图片（毕业证/学位证）一次解析为一份画像。
// 文本与各图片的解析互不依赖，并行执行（errgroup），最后由代码层融合（见 merge.go）。
func (p *ParserAgent) ParseMulti(ctx context.Context, content string, images []types.ImageInput) (*ParseResult, error) {
	// 图片类型前置校验（快速失败，不进并行）
	for _, img := range images {
		if img.Type != types.ImageTypeDiploma {
			return nil, fmt.Errorf("暂不支持的图片类型 %q（当前仅支持毕业证/学位证）", img.Type)
		}
	}

	var (
		textProfile   *UserProfile
		imageProfiles = make([]*UserProfile, len(images))
		uncertain     []UncertainField
		mu            sync.Mutex // 保护 uncertain 并发追加
	)

	g, gctx := errgroup.WithContext(ctx)

	// 文本来源（并行）
	if content != "" {
		g.Go(func() error {
			r, err := p.ParseText(gctx, content)
			if err != nil {
				return err
			}
			textProfile = r.Profile
			if textProfile == nil {
				textProfile = r.ProfilePartial
			}
			mu.Lock()
			uncertain = append(uncertain, r.UncertainFields...)
			mu.Unlock()
			return nil
		})
	}

	// 图片来源（并行，当前仅支持毕业证/学位证；职位表截图属于岗位分析链路，后续迭代）
	for i, img := range images {
		g.Go(func() error {
			r, err := p.ParseImage(gctx, img.Base64, "毕业证/学位证")
			if err != nil {
				return err
			}
			ip := r.Profile
			if ip == nil {
				ip = r.ProfilePartial
			}
			imageProfiles[i] = ip
			mu.Lock()
			uncertain = append(uncertain, r.UncertainFields...)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if textProfile == nil {
		textProfile = &UserProfile{}
	}
	merged, mergeUncertain := mergeProfiles(textProfile, imageProfiles)
	uncertain = append(uncertain, mergeUncertain...)

	// 来源标记
	parsedFrom := "multi"
	if len(images) == 0 {
		parsedFrom = "text"
	} else if content == "" {
		parsedFrom = "image"
	}

	// 规则校验兜底（省份归一、核心字段检查、置信度重算、状态判定）
	result := &ParseResult{
		Status:          StatusSuccess,
		Profile:         merged,
		Confidence:      1.0,
		UncertainFields: uncertain,
		ParsedFrom:      parsedFrom,
	}
	return validateResult(result), nil
}

// ParseText 解析自然语言文本输入。
func (p *ParserAgent) ParseText(ctx context.Context, content string) (*ParseResult, error) {
	if !p.llm.Has(p.channel) {
		// 降级方案：关键词规则解析（无 LLM 时保证接口可用）
		return p.parseTextFallback(content), nil
	}
	resp, err := p.llm.ChatCompletion(ctx, p.channel,
		[]llm.Message{{Role: "user", Content: content}}, &llm.Options{System: p.systemPrompt()})
	if err != nil {
		// LLM 调用失败同样降级，保证接口可用性
		return p.parseTextFallback(content), nil
	}
	return p.buildResult(resp.Content, "text")
}

// ParseImage 解析图片输入（毕业证/学位证），kind 为 human 可读的图片类型名。
// 系统提示词使用证件专用变体（prompt.NameParserDiploma），聚焦学历/专业提取；
// 未配置视觉渠道时使用 Mock 结果演示 need_confirm 流程。
func (p *ParserAgent) ParseImage(ctx context.Context, imageBase64, kind string) (*ParseResult, error) {
	if !p.llm.Has(llm.ChannelVision) {
		return p.parseImageFallback(), nil
	}
	user := p.prompts.Get(prompt.NameParserImageUser)
	if rendered, err := p.prompts.Render(prompt.NameParserImageUser, map[string]any{"Kind": kind}); err == nil {
		user = rendered
	}
	// 证件专用系统提示词（缺失时回退文本解析默认）
	sys := p.prompts.Get(prompt.NameParserDiploma)
	if sys == "" {
		sys = p.systemPrompt()
	}
	out, err := p.llm.ChatWithImage(ctx, sys, user, imageBase64)
	if err != nil {
		return nil, fmt.Errorf("图片解析失败: %w", err)
	}
	return p.buildResult(out, "image")
}

// llmOutput LLM 原始输出结构（与系统提示词约定的 JSON 对应）。
// 小模型输出类型不稳定（confidence 可能是数字或字符串、uncertain_fields 可能是数组或对象），
// 全部用 RawMessage 接收后容错解析。
type llmOutput struct {
	Profile         UserProfile     `json:"profile"`
	Confidence      json.RawMessage `json:"confidence"`
	UncertainFields json.RawMessage `json:"uncertain_fields"`
}

// parseConfidence 容错解析置信度：兼容数字与数字字符串（如 "0.8"）。
func parseConfidence(raw json.RawMessage) float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return v
		}
	}
	return 0
}

// parseUncertainFields 容错解析 uncertain_fields：兼容数组、单个对象、null。
// 小模型输出格式不稳定，不能假设一定是数组。
func parseUncertainFields(raw json.RawMessage) []UncertainField {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var list []UncertainField
	if err := json.Unmarshal(raw, &list); err == nil {
		return list
	}
	// 单个对象 → 包装为单元素数组
	var single UncertainField
	if err := json.Unmarshal(raw, &single); err == nil && single.Field != "" {
		return []UncertainField{single}
	}
	return nil
}

// buildResult 将 LLM 输出构建为 ParseResult：
// 先解析 JSON，再过规则校验层（省份归一、核心字段检查、置信度重算、状态判定）。
func (p *ParserAgent) buildResult(raw, parsedFrom string) (*ParseResult, error) {
	var out llmOutput
	if err := json.Unmarshal([]byte(extractJSON(raw)), &out); err != nil {
		return nil, fmt.Errorf("LLM 输出解析失败: %w", err)
	}

	result := &ParseResult{
		Status:          StatusSuccess,
		Profile:         &out.Profile,
		Confidence:      parseConfidence(out.Confidence),
		UncertainFields: parseUncertainFields(out.UncertainFields),
		ParsedFrom:      parsedFrom,
	}
	// 规则校验兜底：不依赖模型自觉（PRD 2.1 的置信度规则在此强制执行）
	return validateResult(result), nil
}

// extractJSON 从 LLM 输出中提取 JSON 内容（兼容 ```json 代码块包裹的情况）。
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// 剥离 markdown 代码块标记
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	// 截取首个 { 到末个 } 之间的内容，过滤模型多余输出
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// Confirm 确认修正接口逻辑：将用户确认的字段合并进部分条件，返回完整 Profile（PRD 3.2.2）。
func Confirm(partial *UserProfile, confirmed map[string]string) *UserProfile {
	merged := *partial // 拷贝，避免修改缓存中的原对象
	for field, value := range confirmed {
		switch field {
		case types.FieldEducation:
			merged.Education = value
		case types.FieldMajor:
			merged.Major = value
		case types.FieldMajorCategory:
			merged.MajorCategory = value
		case types.FieldPoliticalStatus:
			merged.PoliticalStatus = value
		case types.FieldGender:
			merged.Gender = value
		}
	}
	// 确认后重新映射专业大类（用户修正了专业名时）
	if merged.MajorCategory == "" && merged.Major != "" {
		merged.MajorCategory = majorCategoryOf(merged.Major)
	}
	return &merged
}
