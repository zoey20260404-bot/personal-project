package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient 建立 Redis 连接并验证可用性。
// 定位为通用缓存客户端，供后续业务按需使用（对话热点上下文、查询结果缓存、
// LLM 结果缓存等）；当前无业务依赖，不可用时不影响系统运行。
func NewRedisClient(addr, password string, db int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("连接 Redis 失败: %w", err)
	}
	return client, nil
}
