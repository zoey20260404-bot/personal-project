package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

// 记忆作用域（scope）：解决"session 隔离太死"问题，支持跨会话经验沉淀。
const (
	ScopeSession = "session" // 会话级：仅当前会话可见
	ScopeUser    = "user"    // 用户级：用户画像/偏好，跨会话可见（按 user_id 隔离）
	ScopeAgent   = "agent"   // Agent 级：经验洞察沉淀，该 Agent 的所有任务共享
)

// MemoryRecord 一条长期记忆（会话消息/用户画像/Agent 经验），向量化后存入 pgvector。
type MemoryRecord struct {
	ID        int64   // 自增主键
	AgentName string  // 归属 Agent（记忆隔离：不同 Agent 的记忆互不污染）
	UserID    uint64  // 归属用户（scope=user 时按此隔离）
	SessionID string  // 会话标识（scope=session 时按此隔离）
	Scope     string  // 作用域：session / user / agent
	Role      string  // user / assistant / system / insight（Agent 自主沉淀的经验）
	Content   string  // 原始文本内容
	Score     float64 // 检索时的相似度得分（仅查询返回时有值）
}

// VectorStore 向量存储客户端（PostgreSQL + pgvector）。
// 承载 Agent 长期记忆与 RAG 知识库的语义检索（PRD 1.3 数据层）。
type VectorStore struct {
	pool *pgxpool.Pool
}

// NewVectorStore 连接 PostgreSQL 并初始化 pgvector 扩展与记忆表。
// dim 为向量维度，须与所用 embedding 模型一致（如 bge-m3 为 1024）；
// 若已有 memories 表结构不匹配（维度或记忆隔离列缺失），将重建该表。
func NewVectorStore(ctx context.Context, dsn string, dim int, logger *slog.Logger) (*VectorStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("连接 PostgreSQL 失败: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("PostgreSQL 不可用: %w", err)
	}
	if _, err := pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		return nil, fmt.Errorf("启用 pgvector 扩展失败: %w", err)
	}
	if err := ensureMemoriesTable(ctx, pool, dim, logger); err != nil {
		return nil, err
	}
	return &VectorStore{pool: pool}, nil
}

// memoriesRequiredColumns 记忆表必需的列（缺失任意一个即触发表重建）。
var memoriesRequiredColumns = []string{"agent_name", "user_id", "session_id", "scope"}

// ensureMemoriesTable 建表并校验结构（维度/记忆隔离列），不匹配则重建。
func ensureMemoriesTable(ctx context.Context, pool *pgxpool.Pool, dim int, logger *slog.Logger) error {
	createTable := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS memories (
		id BIGSERIAL PRIMARY KEY,
		agent_name VARCHAR(64) NOT NULL DEFAULT '',
		user_id BIGINT NOT NULL DEFAULT 0,
		session_id VARCHAR(64) NOT NULL DEFAULT '',
		scope VARCHAR(16) NOT NULL DEFAULT 'session',
		role VARCHAR(20) NOT NULL,
		content TEXT NOT NULL,
		embedding vector(%d),
		created_at TIMESTAMPTZ DEFAULT now()
	)`, dim)

	// 已存在时校验结构：向量维度 + 必需列
	var existingDim int
	err := pool.QueryRow(ctx,
		`SELECT atttypmod FROM pg_attribute
		 WHERE attrelid = 'memories'::regclass AND attname = 'embedding'`).Scan(&existingDim)
	needRebuild := false
	if err == nil {
		if existingDim != dim {
			needRebuild = true
			logger.Warn("memories 向量维度不匹配", "old", existingDim, "new", dim)
		}
		for _, col := range memoriesRequiredColumns {
			var cnt int
			_ = pool.QueryRow(ctx,
				`SELECT count(*) FROM information_schema.columns
				 WHERE table_name = 'memories' AND column_name = $1`, col).Scan(&cnt)
			if cnt == 0 {
				needRebuild = true
				logger.Warn("memories 表缺少列", "column", col)
			}
		}
	}
	if needRebuild {
		logger.Warn("memories 表结构已变更，重建（历史记忆清空）")
		if _, err := pool.Exec(ctx, "DROP TABLE memories"); err != nil {
			return fmt.Errorf("重建向量表失败: %w", err)
		}
	}

	if _, err := pool.Exec(ctx, createTable); err != nil {
		return fmt.Errorf("初始化向量库失败: %w", err)
	}
	if _, err := pool.Exec(ctx,
		"CREATE INDEX IF NOT EXISTS idx_memories_agent_scope ON memories(agent_name, scope, session_id, user_id)"); err != nil {
		return fmt.Errorf("创建向量表索引失败: %w", err)
	}
	return nil
}

// AddMemory 写入一条记忆（含向量），按 Agent 命名空间 + 作用域隔离。
func (s *VectorStore) AddMemory(ctx context.Context, rec MemoryRecord, embedding []float32) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO memories (agent_name, user_id, session_id, scope, role, content, embedding)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		rec.AgentName, rec.UserID, rec.SessionID, rec.Scope, rec.Role, rec.Content,
		pgvector.NewVector(embedding))
	return err
}

// SearchMemories 分层召回记忆，作用域语义（feat003 修正）：
//   - agent 级经验：按 agent_name 隔离（各 Agent 的经验互不污染）
//   - user 级画像：只按 user_id（跨 Agent 共享——用户是谁，谁都需要知道）
//   - session 级会话：只按 session_id（跨 Agent 共享——用户视角是在和一个助手对话）
//
// 按余弦距离统一排序（越相似越靠前）。
func (s *VectorStore) SearchMemories(ctx context.Context, agentName string, userID uint64, sessionID string, embedding []float32, topK int) ([]MemoryRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_name, user_id, session_id, scope, role, content, embedding <=> $4 AS distance
		 FROM memories
		 WHERE (scope = 'agent' AND agent_name = $1)
		    OR (scope = 'user' AND user_id = $2)
		    OR (scope = 'session' AND session_id = $3)
		 ORDER BY embedding <=> $4 LIMIT $5`,
		agentName, userID, sessionID, pgvector.NewVector(embedding), topK)
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}
	defer rows.Close()

	var records []MemoryRecord
	for rows.Next() {
		var r MemoryRecord
		var distance float64
		if err := rows.Scan(&r.ID, &r.AgentName, &r.UserID, &r.SessionID, &r.Scope, &r.Role, &r.Content, &distance); err != nil {
			return nil, err
		}
		r.Score = 1 - distance // 余弦距离转相似度得分
		records = append(records, r)
	}
	return records, rows.Err()
}
