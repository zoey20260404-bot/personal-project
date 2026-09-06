// Package svc 提供服务上下文（ServiceContext）：集中构建并持有应用所有依赖。
//
// 设计原则：依赖统一收口——配置、日志、模型渠道、Prompt 库、存储、工具、
// Agent 运行时全部在 NewServiceContext 中装配一次，各组件从 ServiceContext 取用。
// 日志器为单一实例，逐级注入（不使用全局默认 Logger）。
package svc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"ai-start/configs"
	"ai-start/internal/agent"
	"ai-start/internal/api/logic"
	"ai-start/internal/llm"
	"ai-start/internal/logx"
	"ai-start/internal/prompt"
	"ai-start/internal/runtime"
	"ai-start/internal/store"
	"ai-start/internal/tool"
	"ai-start/internal/types"
)

// ServiceContext 应用服务上下文：所有共享依赖的载体。
type ServiceContext struct {
	Config       *config.Config        // 应用配置
	Logger       *slog.Logger          // 日志器（单一实例，逐级注入）
	loggerCloser io.Closer             // 日志文件句柄（退出时关闭，可能为 nil）
	Models       *llm.Manager          // 模型渠道管理器
	Prompts      *prompt.Store         // Prompt 模板库（Go 常量注册表）
	MySQL        *store.MySQLStore     // MySQL（用户/会话/收藏），nil 表示不可用
	Vector       *store.VectorStore    // pgvector（长期记忆/RAG），nil 表示不可用
	Redis        *redis.Client         // 通用缓存客户端（预留给后续业务），nil 表示不可用
	Memory       *agent.Memory         // 长期记忆模块（三层作用域）
	Services     *logic.Services       // 业务服务集合（user/favorite/parse）
	Chat         *logic.ChatService    // 多轮对话编排（feat002）
	ChatBuffer   store.ChatBuffer      // 短期会话缓冲（Redis/内存降级）
	Tools        *tool.Registry        // 工具注册表
	Runtime      *runtime.Runtime      // 多 Agent 运行时（去中心化总线）
	Flows        *runtime.FlowExecutor // 流程编排器
	Supervisor   *agent.Supervisor     // 调度中枢
	JWTSecret    string                // JWT 签名密钥
}

// NewServiceContext 装配所有依赖。外部依赖（MySQL/pgvector/Redis）不可用时
// 自动降级并记日志，不阻断启动。
func NewServiceContext(cfg *config.Config) (*ServiceContext, error) {
	svc := &ServiceContext{Config: cfg}

	// 日志器：单一实例，后续所有组件共用
	logger, closer, err := logx.NewLogger(cfg.Log)
	if err != nil {
		return nil, err
	}
	svc.Logger = logger
	svc.loggerCloser = closer

	// 模型渠道管理器；未配置密钥的渠道自动停用，各 Agent 走降级逻辑
	svc.Models = llm.NewManager(cfg.Models)
	if !svc.Models.Has(llm.ChannelChat) {
		logger.Warn("chat 渠道未配置密钥，条件解析将使用关键词降级方案")
	}
	if !svc.Models.Has(llm.ChannelEmbedding) {
		logger.Warn("embedding 渠道未配置密钥，长期记忆功能停用")
	}

	// Prompt 模板库（Go 常量注册表，见 internal/prompt/prompts_*.go）
	svc.Prompts = prompt.NewStore()

	// MySQL：用户/会话/收藏等结构化业务数据；不可用时跳过持久化
	if ms, err := store.NewMySQLStore(cfg.Store.MySQL.DSN); err != nil {
		logger.Warn("MySQL 不可用，用户数据将不持久化", "err", err)
	} else {
		svc.MySQL = ms
		// 岗位示例数据：表为空时写入（开发演示；真实数据走职位表导入）
		if err := ms.SeedPositions(store.SamplePositions()); err != nil {
			logger.Warn("岗位示例数据写入失败", "err", err)
		}
	}

	// pgvector 向量库：Agent 长期记忆 + RAG 知识库；不可用时记忆功能停用
	initCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if vs, err := store.NewVectorStore(initCtx, cfg.Store.Postgres.DSN, cfg.Models[llm.ChannelEmbedding].Dim, logger); err != nil {
		logger.Warn("pgvector 不可用，长期记忆功能停用", "err", err)
	} else {
		svc.Vector = vs
	}
	cancel()

	// Redis：通用缓存客户端（预留给后续业务：对话热点上下文、查询结果缓存等）
	// 不可用时仅告警，当前无业务依赖，不影响系统运行
	if rc, err := store.NewRedisClient(cfg.Store.Redis.Addr, cfg.Store.Redis.Password, cfg.Store.Redis.DB); err != nil {
		logger.Warn("Redis 不可用，缓存能力停用", "err", err)
	} else {
		svc.Redis = rc
	}

	// 工具注册表（ReAct Agent 通过 function calling 调用，白名单由各 Agent 配置声明）
	// 工具实现为薄壳：业务逻辑委托 logic 层（闭包延迟取 svc.Services，装配顺序无关）
	svc.Tools = tool.NewRegistry()
	svc.Tools.Register(tool.NewQueryPositionsTool(func(ctx context.Context, f store.PositionFilter) ([]store.Position, int64, error) {
		return svc.Services.Position.Query(f)
	}))
	svc.Tools.Register(tool.NewGetProfileTool(func(ctx context.Context, userID uint64) (*types.UserProfile, error) {
		profile, _, err := svc.Services.Parse.GetLatestProfile(userID)
		return profile, err
	}))
	svc.Tools.Register(tool.NewUpdateProfileTool(func(ctx context.Context, userID uint64, text string) (*types.UserProfile, []string, error) {
		return svc.Services.Parse.UpdateFromText(ctx, userID, text)
	}))
	svc.Tools.Register(tool.NewAddFavoriteTool(func(ctx context.Context, userID uint64, positionID string) error {
		_, err := svc.Services.Favorite.Add(userID, store.Favorite{PositionID: positionID})
		return err
	}))

	// 多 Agent 运行时（去中心化总线架构）+ 流程编排器
	rt, err := agent.BuildRuntime(cfg.Agents, cfg.Runtime, svc.Models, svc.Prompts, svc.Tools, logger)
	if err != nil {
		return nil, err
	}
	svc.Runtime = rt
	svc.Flows = runtime.NewFlowExecutor(rt, cfg.Flows, time.Duration(cfg.Runtime.CallTimeout)*time.Second)
	svc.Supervisor = agent.NewSupervisor(svc.Flows)
	svc.Memory = agent.NewMemory(svc.Models, svc.Vector, logger)

	// JWT 密钥：未配置时生成随机密钥（重启后旧 token 失效，仅开发用）
	svc.JWTSecret = cfg.JWT.Secret
	if svc.JWTSecret == "" {
		svc.JWTSecret = randomHex(16)
		logger.Warn("未配置 JWT_SECRET，已生成随机密钥（重启后已有 token 失效）")
	}

	// 业务服务层：Handler 经此调用业务逻辑（api 层不直接触碰存储）
	svc.Services = logic.NewServices(svc.MySQL, svc.Supervisor, svc.JWTSecret, cfg.JWT.ExpireHours, logger)

	// 短期会话缓冲：Redis 优先，不可用降级内存
	if svc.Redis != nil {
		svc.ChatBuffer = store.NewRedisChatBuffer(svc.Redis)
	} else {
		svc.ChatBuffer = store.NewMemoryChatBuffer()
	}
	// 多轮对话编排（feat002：路由 + ReAct + 记忆）
	svc.Chat = logic.NewChatService(svc.Runtime, svc.Memory, svc.ChatBuffer, svc.Services.Parse, logger)

	return svc, nil
}

// Close 释放服务上下文持有的资源（日志文件句柄等）。
func (s *ServiceContext) Close() {
	if s.loggerCloser != nil {
		_ = s.loggerCloser.Close()
	}
}

// randomHex 生成 n 字节的随机十六进制字符串（JWT 临时密钥用）。
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
