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

func (NoopPresenceTracker) AddConnection(conn *WSConnection) error    { return nil }
func (NoopPresenceTracker) TouchConnection(conn *WSConnection) error  { return nil }
func (NoopPresenceTracker) RemoveConnection(conn *WSConnection) error { return nil }

// RedisPresenceStore 使用 Redis TTL key 保存全局在线连接快照。
// 它不保存真实 WebSocket 连接，真实连接仍由每个进程自己的 WSHub 管理。
type RedisPresenceStore struct {
	client     *redis.Client
	keyPrefix  string
	ttl        time.Duration
	instanceID string
}

func NewRedisPresenceStore(client *redis.Client, cfg RedisConfig) *RedisPresenceStore {
	return &RedisPresenceStore{
		client:     client,
		keyPrefix:  strings.TrimRight(cfg.KeyPrefix, ":"),
		ttl:        cfg.PresenceTTL,
		instanceID: cfg.InstanceID,
	}
}

func (s *RedisPresenceStore) AddConnection(conn *WSConnection) error {
	return s.writeConnection(conn)
}

func (s *RedisPresenceStore) TouchConnection(conn *WSConnection) error {
	return s.writeConnection(conn)
}

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

func (s *RedisPresenceStore) connectionKey(userID int64, connectionID string) string {
	return fmt.Sprintf("%s:presence:user:%d:conn:%s", s.keyPrefix, userID, connectionID)
}

func (s *RedisPresenceStore) connectionsKey(userID int64) string {
	return fmt.Sprintf("%s:presence:user:%d:connections", s.keyPrefix, userID)
}
