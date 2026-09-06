package logic

import (
	"ai-start/internal/store"
)

// PositionService 岗位查询域业务逻辑（Researcher 初版：纯规则过滤，无 AI）。
type PositionService struct {
	mysql *store.MySQLStore
}

// NewPositionService 创建岗位查询服务。
func NewPositionService(mysql *store.MySQLStore) *PositionService {
	return &PositionService{mysql: mysql}
}

// Query 按条件分页查询岗位。存储不可用时返回空列表（页面展示场景，降级友好）。
func (s *PositionService) Query(filter store.PositionFilter) ([]store.Position, int64, error) {
	if s.mysql == nil {
		return []store.Position{}, 0, nil
	}
	return s.mysql.QueryPositions(filter)
}
