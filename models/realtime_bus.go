package models

import (
	"encoding/json"
	"log"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

// MessageDeliverEvent 是实时投递时传递的最小事件。
// 当前项目采用单进程 Hub 投递；该结构用于让投递数据在代码内保持清晰边界。
type MessageDeliverEvent struct {
	UserIDs []int64    `json:"user_ids"`
	Message MessageDTO `json:"message"`
}

// RealtimeBus 抽象实时事件总线。
// 目前只提供内存实现，不接入外部消息组件，业务入口仍然直接依赖稳定接口。
type RealtimeBus interface {
	PublishMessageDeliver(event MessageDeliverEvent) error
	SubscribeMessageDeliver(handler func(MessageDeliverEvent)) error
}

// MemoryRealtimeBus 是单进程内存实现，主要用于开发和当前 v7 落地占位。
type MemoryRealtimeBus struct {
	mu       sync.RWMutex
	handlers []func(MessageDeliverEvent)
}

// NewMemoryRealtimeBus 创建单进程内存事件总线。
// 没有启用 Redis 时，路由初始化会使用它把消息投递事件转回本机 Hub。
func NewMemoryRealtimeBus() *MemoryRealtimeBus {
	return &MemoryRealtimeBus{}
}

// PublishMessageDeliver 发布一条消息投递事件。
// 内存实现会同步调用所有订阅者；Redis 实现则发布到 Pub/Sub channel。
func (b *MemoryRealtimeBus) PublishMessageDeliver(event MessageDeliverEvent) error {
	// 先复制 handler 列表再调用，避免 handler 内部再次订阅时造成锁重入或长时间持锁。
	b.mu.RLock()
	handlers := append([]func(MessageDeliverEvent){}, b.handlers...)
	b.mu.RUnlock()

	// 内存实现同步执行 handler，便于本地调试和单进程部署。
	for _, handler := range handlers {
		handler(event)
	}
	return nil
}

// SubscribeMessageDeliver 注册消息投递事件处理器。
// 当前应用只注册一个处理器：把事件投递到本实例的 WebSocket Hub。
func (b *MemoryRealtimeBus) SubscribeMessageDeliver(handler func(MessageDeliverEvent)) error {
	// nil handler 没有业务意义，直接忽略，调用方不需要额外判空。
	if handler == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, handler)
	return nil
}

// RedisRealtimeBus 使用 Redis Pub/Sub 分发跨节点实时投递事件。
// 每个实例都订阅同一个 channel，收到事件后只投递自己本机 Hub 中存在的连接。
type RedisRealtimeBus struct {
	client  *redis.Client
	channel string
}

// NewRedisRealtimeBus 创建 Redis Pub/Sub 实时总线。
// channel 为空时使用默认频道，避免配置缺项导致实时投递静默失效。
func NewRedisRealtimeBus(client *redis.Client, channel string) *RedisRealtimeBus {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = "flow-talk:message_deliver"
	}
	return &RedisRealtimeBus{
		client:  client,
		channel: channel,
	}
}

// PublishMessageDeliver 把消息投递事件序列化后发布到 Redis。
// 所有实例都会收到该事件，但每个实例只会投递自己 Hub 里的本机连接。
func (b *RedisRealtimeBus) PublishMessageDeliver(event MessageDeliverEvent) error {
	if b == nil || b.client == nil {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return b.client.Publish(redisContext(), b.channel, payload).Err()
}

// SubscribeMessageDeliver 订阅 Redis 实时投递频道。
// 订阅成功后启动后台 goroutine 持续消费消息，避免阻塞路由初始化。
func (b *RedisRealtimeBus) SubscribeMessageDeliver(handler func(MessageDeliverEvent)) error {
	if b == nil || b.client == nil || handler == nil {
		return nil
	}

	pubsub := b.client.Subscribe(redisContext(), b.channel)
	if _, err := pubsub.Receive(redisContext()); err != nil {
		_ = pubsub.Close()
		return err
	}

	go func() {
		defer func() {
			if err := pubsub.Close(); err != nil {
				log.Printf("关闭 Redis 实时订阅失败: %v", err)
			}
		}()

		for message := range pubsub.Channel() {
			var event MessageDeliverEvent
			if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
				log.Printf("解析 Redis 实时投递事件失败: %v", err)
				continue
			}
			handler(event)
		}
	}()
	return nil
}
