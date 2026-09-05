// Package llm 模型接入层：以"渠道"为粒度管理 OpenAI 兼容协议的模型连接。
//
// 设计原则：模型不绑定代码——渠道在配置中声明（可指向智谱/硅基流动/DeepSeek/本地 Ollama 等），
// Agent 按渠道名引用，单次调用还可临时覆盖 model/temperature 等参数。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	openai "github.com/sashabaranov/go-openai"

	"ai-start/configs"
)

// 约定渠道名（各模块默认引用，可在配置中新增任意渠道）。
const (
	ChannelChat      = "chat"      // 文本对话
	ChannelVision    = "vision"    // 视觉理解（密钥为空时复用 chat）
	ChannelEmbedding = "embedding" // 向量化
)

// Message 对话消息（跨渠道统一结构，与具体 SDK 解耦）。
type Message struct {
	Role       string     // system / user / assistant / tool
	Content    string     // 文本内容
	ToolCalls  []ToolCall // assistant 消息中的工具调用请求
	ToolCallID string     // tool 消息对应的调用 ID
}

// ToolCall 模型发起的工具调用。
type ToolCall struct {
	ID        string // 调用 ID（回传工具结果时需带上）
	Name      string // 工具名
	Arguments string // 调用参数（JSON 字符串）
}

// ToolDef 工具定义（function calling 描述）。
type ToolDef struct {
	Name        string // 工具名
	Description string // 功能描述（模型据此决定是否调用）
	ParamsJSON  string // 参数 JSON Schema
}

// Options 单次调用的可选参数（nil 字段使用渠道配置）。
type Options struct {
	System      string    // 系统提示词
	Tools       []ToolDef // 可用工具（function calling），为空则不启用
	Model       string    // 临时覆盖模型名（为空用渠道配置）
	Temperature *float64  // 临时覆盖采样温度（nil 用渠道配置）
}

// Manager 多渠道模型管理器：按渠道名持有连接，为各 Agent 提供统一调用入口。
// 某渠道未配置密钥时连接为 nil，调用方应按 Has 检查降级。
type Manager struct {
	clients map[string]*openai.Client     // 渠道名 → 连接
	configs map[string]config.ModelConfig // 渠道名 → 渠道配置（模型名/温度等）
}

// NewManager 按配置创建多渠道管理器。vision 渠道密钥为空时自动复用 chat 渠道。
func NewManager(models map[string]config.ModelConfig) *Manager {
	m := &Manager{
		clients: make(map[string]*openai.Client),
		configs: make(map[string]config.ModelConfig),
	}
	for name, cfg := range models {
		if cfg.APIKey == "" {
			continue // 未配置密钥的渠道不建连接
		}
		m.clients[name] = newOpenAIClient(cfg.BaseURL, cfg.APIKey)
		m.configs[name] = cfg
	}
	// vision 回落到 chat
	if _, ok := m.clients[ChannelVision]; !ok {
		if client, ok := m.clients[ChannelChat]; ok {
			m.clients[ChannelVision] = client
			m.configs[ChannelVision] = m.configs[ChannelChat]
		}
	}
	return m
}

// newOpenAIClient 创建 OpenAI 兼容协议客户端。
func newOpenAIClient(baseURL, apiKey string) *openai.Client {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	return openai.NewClientWithConfig(cfg)
}

// Has 渠道是否可用（已配置密钥）。
func (m *Manager) Has(channel string) bool {
	if m == nil {
		return false
	}
	_, ok := m.clients[channel]
	return ok
}

// Chat 文本对话快捷入口（chat 渠道，使用渠道默认参数）。
func (m *Manager) Chat(ctx context.Context, system, user string) (string, error) {
	resp, err := m.ChatCompletion(ctx, ChannelChat,
		[]Message{{Role: "user", Content: user}}, &Options{System: system})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ChatCompletion 通用对话接口：指定渠道，支持系统提示词、工具与参数临时覆盖。
func (m *Manager) ChatCompletion(ctx context.Context, channel string, messages []Message, opts *Options) (*Message, error) {
	client, ok := m.clients[channel]
	if !ok {
		return nil, fmt.Errorf("模型渠道 %q 未配置", channel)
	}
	channelCfg := m.configs[channel]

	req := openai.ChatCompletionRequest{Model: channelCfg.Model}
	// 单次调用参数覆盖（温度/模型）
	if opts != nil {
		if opts.Model != "" {
			req.Model = opts.Model
		}
		if opts.Temperature != nil {
			req.Temperature = float32(*opts.Temperature)
		} else if channelCfg.Temperature != nil {
			req.Temperature = float32(*channelCfg.Temperature)
		}
		if opts.System != "" {
			req.Messages = append(req.Messages, openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleSystem, Content: opts.System,
			})
		}
	} else if channelCfg.Temperature != nil {
		req.Temperature = float32(*channelCfg.Temperature)
	}
	if channelCfg.MaxTokens > 0 {
		req.MaxTokens = channelCfg.MaxTokens
	}

	for _, msg := range messages {
		req.Messages = append(req.Messages, toOpenAIMessage(msg))
	}
	// 挂载工具定义
	if opts != nil {
		for _, t := range opts.Tools {
			var params map[string]interface{}
			if err := json.Unmarshal([]byte(t.ParamsJSON), &params); err != nil {
				return nil, fmt.Errorf("工具 %q 参数 Schema 解析失败: %w", t.Name, err)
			}
			req.Tools = append(req.Tools, openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  params,
				},
			})
		}
	}

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("渠道 %q 调用失败: %w", channel, err)
	}
	return fromOpenAIMessage(resp.Choices[0].Message), nil
}

// ChatWithImage 图文对话（vision 渠道），imageBase64 为图片的 Base64 编码内容。
func (m *Manager) ChatWithImage(ctx context.Context, system, user, imageBase64 string) (string, error) {
	client, ok := m.clients[ChannelVision]
	if !ok {
		return "", errors.New("视觉渠道未配置")
	}
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: m.configs[ChannelVision].Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: system},
			{
				Role: openai.ChatMessageRoleUser,
				MultiContent: []openai.ChatMessagePart{
					{Type: openai.ChatMessagePartTypeText, Text: user},
					{
						Type: openai.ChatMessagePartTypeImageURL,
						ImageURL: &openai.ChatMessageImageURL{
							// data URL 形式内联图片，兼容各家视觉模型
							URL: "data:image/png;base64," + imageBase64,
						},
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("视觉模型调用失败: %w", err)
	}
	return resp.Choices[0].Message.Content, nil
}

// Embed 文本向量化（embedding 渠道），用于记忆与知识库的语义检索。
func (m *Manager) Embed(ctx context.Context, text string) ([]float32, error) {
	client, ok := m.clients[ChannelEmbedding]
	if !ok {
		return nil, errors.New("向量化渠道未配置")
	}
	resp, err := client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(m.configs[ChannelEmbedding].Model),
		Input: []string{text},
	})
	if err != nil {
		return nil, fmt.Errorf("向量化失败: %w", err)
	}
	return resp.Data[0].Embedding, nil
}

// toOpenAIMessage 转换为 go-openai 消息结构。
func toOpenAIMessage(msg Message) openai.ChatCompletionMessage {
	out := openai.ChatCompletionMessage{
		Role:       msg.Role,
		Content:    msg.Content,
		ToolCallID: msg.ToolCallID,
	}
	for _, tc := range msg.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, openai.ToolCall{
			ID:       tc.ID,
			Type:     openai.ToolTypeFunction,
			Function: openai.FunctionCall{Name: tc.Name, Arguments: tc.Arguments},
		})
	}
	return out
}

// fromOpenAIMessage 从 go-openai 消息结构转换。
func fromOpenAIMessage(msg openai.ChatCompletionMessage) *Message {
	out := &Message{Role: msg.Role, Content: msg.Content}
	for _, tc := range msg.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out
}
