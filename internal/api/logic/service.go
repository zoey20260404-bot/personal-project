// Package service 业务逻辑层：介于 HTTP 接口层（api）与存储层（store）之间。
// 职责：业务规则、数据组装、事务编排；不感知 HTTP（无 gin 依赖），不直接写 SQL。
package logic

import (
	"log/slog"

	"ai-start/internal/agent"
	"ai-start/internal/runtime"
	"ai-start/internal/store"
)

// Services 业务服务集合：统一装配于 svc.ServiceContext，Handler 按域取用。
type Services struct {
	User     *UserService     // 用户注册/登录/令牌
	Favorite *FavoriteService // 岗位收藏
	Parse    *ParseService    // 条件解析编排
	Position *PositionService // 岗位查询（Researcher 初版）
	Advise   *AdviseService   // 选岗推荐（feat004 冲稳保主链路）
}

// NewServices 装配全部业务服务。
// 依赖为具体存储/调度组件，由 svc 层注入；mysql 为 nil 时相关能力降级。
// memory 用于规则触发的用户记忆沉淀（档案变更/收藏等明确信号）。
func NewServices(mysql *store.MySQLStore, supervisor *agent.Supervisor, flows *runtime.FlowExecutor, jwtSecret string, jwtExpireHours int, logger *slog.Logger, memory *agent.Memory) *Services {
	parse := NewParseService(supervisor, mysql, logger, memory)
	return &Services{
		User:     NewUserService(mysql, jwtSecret, jwtExpireHours),
		Favorite: NewFavoriteService(mysql, memory),
		Parse:    parse,
		Position: NewPositionService(mysql),
		Advise:   NewAdviseService(flows, mysql, parse, logger),
	}
}
