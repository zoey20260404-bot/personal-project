// Package llm 封装大模型调用能力。
// 对上提供统一接口，屏蔽不同模型提供方（OpenAI、通义千问等）的差异。
package llm

// Client 大模型客户端接口，各 Provider 需实现该接口。
type Client interface {
	// Chat 发起一轮对话，messages 为按时间序的消息列表。
	Chat(messages []Message) (string, error)
}

// Message 对话消息结构。
type Message struct {
	Role    string // 角色：system / user / assistant
	Content string // 消息内容
}
