package models

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// PresenceTracker 记录 WebSocket 连接生命周期。内存模式下 Hub 自身就是状态来源；
// Redis 模式下需要额外把连接快照写入 Redis，供其它实例查询。
type PresenceTracker interface {
	AddConnection(conn *WSConnection) error
	TouchConnection(conn *WSConnection) error
	RemoveConnection(conn *WSConnection) error
}

type NoopPresenceTracker struct{}

// AddConnection 在内存模式下不需要额外记录连接状态。
func (NoopPresenceTracker) AddConnection(conn *WSConnection) error { return nil }

// TouchConnection 在内存模式下由 WSHub 自己维护 LastActiveAt。
func (NoopPresenceTracker) TouchConnection(conn *WSConnection) error { return nil }

// RemoveConnection 在内存模式下由 WSHub 删除连接即可。
func (NoopPresenceTracker) RemoveConnection(conn *WSConnection) error { return nil }

// RedisPresenceStore 使用 Redis TTL key 保存全局在线连接快照。
// 它不保存真实 WebSocket 连接，真实连接仍由每个进程自己的 WSHub 管理。
type RedisPresenceStore struct {
	client     *redis.Client
	keyPrefix  string
	ttl        time.Duration
	instanceID string
}

// NewRedisPresenceStore 创建 Redis 在线状态存储。
// keyPrefix 会去掉末尾冒号，避免拼 key 时出现重复分隔符。
func NewRedisPresenceStore(client *redis.Client, cfg RedisConfig) *RedisPresenceStore {
	return &RedisPresenceStore{
		client:     client,
		keyPrefix:  strings.TrimRight(cfg.KeyPrefix, ":"),
		ttl:        cfg.PresenceTTL,
		instanceID: cfg.InstanceID,
	}
}

// AddConnection 记录一条新 WebSocket 连接，并设置 TTL。
func (s *RedisPresenceStore) AddConnection(conn *WSConnection) error {
	return s.writeConnection(conn)
}

// TouchConnection 刷新连接快照和 TTL，用于心跳或任意客户端事件。
func (s *RedisPresenceStore) TouchConnection(conn *WSConnection) error {
	return s.writeConnection(conn)
}

// RemoveConnection 删除连接快照，并从用户连接集合中移除该连接 ID。
func (s *RedisPresenceStore) RemoveConnection(conn *WSConnection) error {
	if s == nil || s.client == nil || conn == nil {
		return nil
	}
	ctx := redisContext()
	if err := s.client.Del(ctx, s.connectionKey(conn.UserID, conn.ID)).Err(); err != nil {
		return err
	}
	return s.client.ZRem(ctx, s.connectionsKey(conn.UserID), conn.ID).Err()
}

// Presence 汇总 Redis 中某个用户的连接快照。
// 连接 hash 可能已经因为 TTL 过期，因此会顺手清理 ZSET 中的陈旧 connection_id。
func (s *RedisPresenceStore) Presence(userID int64) (PresenceDTO, error) {
	if userID <= 0 {
		return PresenceDTO{}, ErrInvalidMember
	}
	if s == nil || s.client == nil {
		return PresenceDTO{UserID: userID}, nil
	}

	ctx := redisContext()
	connectionIDs, err := s.client.ZRange(ctx, s.connectionsKey(userID), 0, -1).Result()
	if err != nil {
		return PresenceDTO{}, err
	}

	var lastActive time.Time
	connectionCount := 0
	staleConnectionIDs := make([]interface{}, 0)
	for _, connectionID := range connectionIDs {
		key := s.connectionKey(userID, connectionID)
		values, err := s.client.HGetAll(ctx, key).Result()
		if err != nil {
			return PresenceDTO{}, err
		}
		if len(values) == 0 {
			staleConnectionIDs = append(staleConnectionIDs, connectionID)
			continue
		}

		connectionCount++
		if parsedAt, err := strconv.ParseInt(values["last_active_unix"], 10, 64); err == nil {
			activeAt := time.Unix(parsedAt, 0)
			if activeAt.After(lastActive) {
				lastActive = activeAt
			}
		}
	}
	if len(staleConnectionIDs) > 0 {
		_ = s.client.ZRem(ctx, s.connectionsKey(userID), staleConnectionIDs...).Err()
	}

	return PresenceDTO{
		UserID:          userID,
		Online:          connectionCount > 0,
		ConnectionCount: connectionCount,
		LastActiveAt:    formatOptionalTime(lastActive),
	}, nil
}

// BatchPresence 批量读取 Redis 在线状态。
// 返回顺序遵循去重排序后的 userIDs，便于前端和测试稳定比对。
func (s *RedisPresenceStore) BatchPresence(userIDs []int64) ([]PresenceDTO, error) {
	userIDs = uniquePositiveIDs(userIDs)
	if len(userIDs) == 0 {
		return nil, ErrInvalidMember
	}

	result := make([]PresenceDTO, 0, len(userIDs))
	for _, userID := range userIDs {
		presence, err := s.Presence(userID)
		if err != nil {
			return nil, err
		}
		result = append(result, presence)
	}
	return result, nil
}

// writeConnection 写入或刷新单条连接快照。
// hash 保存连接详情，ZSET 保存用户所有连接 ID 和最近活跃排序，两者配合完成在线状态聚合。
func (s *RedisPresenceStore) writeConnection(conn *WSConnection) error {
	if s == nil || s.client == nil || conn == nil {
		return nil
	}

	now := time.Now()
	conn.LastActiveAt = now
	ctx := redisContext()
	key := s.connectionKey(conn.UserID, conn.ID)
	values := map[string]any{
		"user_id":          strconv.FormatInt(conn.UserID, 10),
		"connection_id":    conn.ID,
		"device_id":        conn.DeviceID,
		"instance_id":      s.instanceID,
		"last_active_at":   now.Format(time.RFC3339),
		"last_active_unix": strconv.FormatInt(now.Unix(), 10),
	}
	if err := s.client.HSet(ctx, key, values).Err(); err != nil {
		return err
	}
	if err := s.client.Expire(ctx, key, s.ttl).Err(); err != nil {
		return err
	}
	if err := s.client.ZAdd(ctx, s.connectionsKey(conn.UserID), redis.Z{
		Score:  float64(now.Unix()),
		Member: conn.ID,
	}).Err(); err != nil {
		return err
	}
	return s.client.Expire(ctx, s.connectionsKey(conn.UserID), s.ttl*2).Err()
}

// connectionKey 是单条连接详情 hash 的 Redis key。
func (s *RedisPresenceStore) connectionKey(userID int64, connectionID string) string {
	return fmt.Sprintf("%s:presence:user:%d:conn:%s", s.keyPrefix, userID, connectionID)
}

// connectionsKey 是某个用户全部连接 ID 的 ZSET key。
func (s *RedisPresenceStore) connectionsKey(userID int64) string {
	return fmt.Sprintf("%s:presence:user:%d:connections", s.keyPrefix, userID)
}
