package store

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ChatMessage 一条对话消息（短期记忆）。
type ChatMessage struct {
	Role    string `json:"role"`    // user / assistant
	Content string `json:"content"` // 消息内容
}

// ChatBuffer 短期会话缓冲：保存最近 N 轮对话（与 pgvector 长期记忆互补）。
type ChatBuffer interface {
	// Append 追加一条消息。
	Append(ctx context.Context, sessionID, role, content string) error
	// Recent 取最近 limit 条消息（时间正序）。
	Recent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error)
}

// chatBufferTTL 会话缓冲有效期（短期记忆，超时自动清除）。
const chatBufferTTL = 2 * time.Hour

// chatBufferMaxLen 会话缓冲最大长度（超出截断最旧消息）。
const chatBufferMaxLen = 20

// RedisChatBuffer Redis 实现的会话缓冲（list 结构：尾部追加、左侧截断）。
type RedisChatBuffer struct {
	client *redis.Client
}

// NewRedisChatBuffer 创建 Redis 会话缓冲。
func NewRedisChatBuffer(client *redis.Client) *RedisChatBuffer {
	return &RedisChatBuffer{client: client}
}

func chatBufferKey(sessionID string) string { return "chat:buf:" + sessionID }

// Append 追加消息到会话缓冲（自动截断与续期）。
func (b *RedisChatBuffer) Append(ctx context.Context, sessionID, role, content string) error {
	data, err := json.Marshal(ChatMessage{Role: role, Content: content})
	if err != nil {
		return err
	}
	key := chatBufferKey(sessionID)
	pipe := b.client.TxPipeline()
	pipe.RPush(ctx, key, data)
	pipe.LTrim(ctx, key, -chatBufferMaxLen, -1) // 只保留最近 N 条
	pipe.Expire(ctx, key, chatBufferTTL)
	_, err = pipe.Exec(ctx)
	return err
}

// Recent 读取最近 limit 条消息。
func (b *RedisChatBuffer) Recent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	if limit <= 0 {
		limit = 10
	}
	items, err := b.client.LRange(ctx, chatBufferKey(sessionID), -int64(limit), -1).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var messages []ChatMessage
	for _, item := range items {
		var msg ChatMessage
		if json.Unmarshal([]byte(item), &msg) == nil {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

// MemoryChatBuffer 内存实现的会话缓冲（Redis 不可用时降级）。
type MemoryChatBuffer struct {
	mu    sync.RWMutex
	items map[string][]ChatMessage
}

// NewMemoryChatBuffer 创建内存会话缓冲。
func NewMemoryChatBuffer() *MemoryChatBuffer {
	return &MemoryChatBuffer{items: make(map[string][]ChatMessage)}
}

// Append 追加消息到内存缓冲。
func (b *MemoryChatBuffer) Append(_ context.Context, sessionID, role, content string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	msgs := append(b.items[sessionID], ChatMessage{Role: role, Content: content})
	if len(msgs) > chatBufferMaxLen {
		msgs = msgs[len(msgs)-chatBufferMaxLen:]
	}
	b.items[sessionID] = msgs
	return nil
}

// Recent 读取最近 limit 条消息。
func (b *MemoryChatBuffer) Recent(_ context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	msgs := b.items[sessionID]
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	return append([]ChatMessage{}, msgs...), nil
}
