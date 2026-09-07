package logic

import (
	"context"
	"errors"

	"ai-start/internal/agent"
	"ai-start/internal/store"
)

// 收藏域错误。
var (
	ErrFavoriteStoreOffline = errors.New("收藏存储不可用")
	ErrFavoriteNotFound     = errors.New("收藏记录不存在")
	ErrInvalidAction        = errors.New("action 仅支持 add / remove")
)

// FavoriteService 收藏域业务逻辑。
type FavoriteService struct {
	mysql  *store.MySQLStore
	memory *agent.Memory // 长期记忆（收藏行为沉淀为用户画像，feat003）
}

// NewFavoriteService 创建收藏服务。
func NewFavoriteService(mysql *store.MySQLStore, memory *agent.Memory) *FavoriteService {
	return &FavoriteService{mysql: mysql, memory: memory}
}

// Add 收藏岗位；重复收藏幂等（created=false）。
// 收藏成功时沉淀一条 user 作用域记忆（用户偏好信号）。
func (s *FavoriteService) Add(userID uint64, fav store.Favorite) (created bool, err error) {
	if s.mysql == nil {
		return false, ErrFavoriteStoreOffline
	}
	fav.UserID = userID
	created, err = s.mysql.AddFavorite(&fav)
	if err == nil && created && s.memory != nil {
		s.memory.Remember(context.Background(), store.MemoryRecord{
			UserID:  userID,
			Scope:   store.ScopeUser,
			Role:    "insight",
			Content: "用户收藏了岗位 " + fav.PositionID + "（" + fav.Category + "类）",
		})
	}
	return created, err
}

// Remove 取消收藏；记录不存在返回 ErrFavoriteNotFound。
func (s *FavoriteService) Remove(userID uint64, positionID string) error {
	if s.mysql == nil {
		return ErrFavoriteStoreOffline
	}
	affected, err := s.mysql.RemoveFavorite(userID, positionID)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrFavoriteNotFound
	}
	return nil
}

// List 查询用户收藏列表（按创建时间倒序）。
func (s *FavoriteService) List(userID uint64) ([]store.Favorite, error) {
	if s.mysql == nil {
		return nil, ErrFavoriteStoreOffline
	}
	return s.mysql.ListFavorites(userID)
}
