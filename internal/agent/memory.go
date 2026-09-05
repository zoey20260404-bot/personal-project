package agent

import (
	"context"
	"log/slog"

	"ai-start/internal/llm"
	"ai-start/internal/store"
)

// Memory Agent 长期记忆：消息/画像/经验向量化后存入 pgvector。
//
// 三层作用域（解决"session 隔离太死"问题，支持跨会话经验沉淀）：
//   - session：会话消息，仅当前会话可见
//   - user：用户画像/偏好（如"党员、应届、计算机专业"），跨会话沉淀，按用户隔离
//   - agent：Agent 自主沉淀的经验洞察（如"广东税务党员+应届岗竞争比低40%"），
//     该 Agent 的所有任务共享，新会话也能复用
//
// 与短期缓存（Redis 解析会话）区分：记忆是持久的、可检索的。
type Memory struct {
	llm    *llm.Manager       // 模型管理器（使用 embedding 渠道），不可用时记忆功能停用
	vector *store.VectorStore // 向量存储，nil 时记忆功能停用
	logger *slog.Logger       // 日志器（注入）
}

// NewMemory 创建记忆模块；任一依赖不可用时退化为空操作（不报错）。
func NewMemory(client *llm.Manager, vector *store.VectorStore, logger *slog.Logger) *Memory {
	return &Memory{llm: client, vector: vector, logger: logger}
}

// Enabled 记忆功能是否可用（需要 embedding 渠道 + 向量库同时可用）。
func (m *Memory) Enabled() bool {
	return m.llm != nil && m.llm.Has(llm.ChannelEmbedding) && m.vector != nil
}

// Remember 写入一条记忆。
// scope 决定可见范围：store.ScopeSession / store.ScopeUser / store.ScopeAgent。
// 向量化失败时仅记日志不中断主流程（记忆是增强能力，不应阻塞业务）。
func (m *Memory) Remember(ctx context.Context, rec store.MemoryRecord) {
	if !m.Enabled() || rec.Content == "" {
		return
	}
	if rec.Scope == "" {
		rec.Scope = store.ScopeSession
	}
	embedding, err := m.llm.Embed(ctx, rec.Content)
	if err != nil {
		m.logger.Warn("记忆向量化失败（跳过）", "err", err)
		return
	}
	if err := m.vector.AddMemory(ctx, rec, embedding); err != nil {
		m.logger.Warn("记忆写入失败（跳过）", "err", err)
	}
}

// Recall 分层召回记忆：合并 agent 级经验 + 当前用户画像 + 当前会话消息，
// 按语义相似度统一排序。不可用时返回空切片，调用方按"无历史上下文"处理。
func (m *Memory) Recall(ctx context.Context, agentName string, userID uint64, sessionID, query string, topK int) []store.MemoryRecord {
	if !m.Enabled() || query == "" {
		return nil
	}
	embedding, err := m.llm.Embed(ctx, query)
	if err != nil {
		m.logger.Warn("召回向量化失败（跳过）", "err", err)
		return nil
	}
	records, err := m.vector.SearchMemories(ctx, agentName, userID, sessionID, embedding, topK)
	if err != nil {
		m.logger.Warn("记忆召回失败（跳过）", "err", err)
		return nil
	}
	return records
}
