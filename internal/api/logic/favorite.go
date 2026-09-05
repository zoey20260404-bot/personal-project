package logic

import (
	"errors"

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
	mysql *store.MySQLStore
}

// NewFavoriteService 创建收藏服务。
func NewFavoriteService(mysql *store.MySQLStore) *FavoriteService {
	return &FavoriteService{mysql: mysql}
}

// Add 收藏岗位；重复收藏幂等（created=false）。
func (s *FavoriteService) Add(userID uint64, fav store.Favorite) (created bool, err error) {
	if s.mysql == nil {
		return false, ErrFavoriteStoreOffline
	}
	fav.UserID = userID
	return s.mysql.AddFavorite(&fav)
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
