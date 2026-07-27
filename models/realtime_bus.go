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

// PresenceChangedEvent 在用户连接数量变化时广播最新的权威在线状态。
type PresenceChangedEvent struct {
	Presence PresenceDTO `json:"presence"`
}

// ConversationUnreadChangedEvent 只投递给 UserID 对应用户的全部在线连接。
type ConversationUnreadChangedEvent struct {
	UserID int64                      `json:"user_id"`
	State  ConversationUnreadStateDTO `json:"state"`
}

// ConversationChangedEvent 把会话变化定向投递给受影响用户。
type ConversationChangedEvent struct {
	UserIDs []int64                    `json:"user_ids"`
	Change  ConversationChangedPayload `json:"change"`
}

// RealtimeBus 抽象消息投递、在线状态、会话未读和会话变化四类实时事件。
// 单实例使用内存实现，多实例使用 Redis Pub/Sub，业务入口只依赖这组稳定接口。
type RealtimeBus interface {
	PublishMessageDeliver(event MessageDeliverEvent) error
	SubscribeMessageDeliver(handler func(MessageDeliverEvent)) error
	PublishPresenceChanged(event PresenceChangedEvent) error
	SubscribePresenceChanged(handler func(PresenceChangedEvent)) error
	PublishConversationUnreadChanged(event ConversationUnreadChangedEvent) error
	SubscribeConversationUnreadChanged(handler func(ConversationUnreadChangedEvent)) error
	PublishConversationChanged(event ConversationChangedEvent) error
	SubscribeConversationChanged(handler func(ConversationChangedEvent)) error
}

// MemoryRealtimeBus 是单进程内存实现，主要用于开发和当前 v7 落地占位。
type MemoryRealtimeBus struct {
	mu                          sync.RWMutex
	messageHandlers             []func(MessageDeliverEvent)
	presenceHandlers            []func(PresenceChangedEvent)
	conversationUnreadHandlers  []func(ConversationUnreadChangedEvent)
	conversationChangedHandlers []func(ConversationChangedEvent)
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
	handlers := append([]func(MessageDeliverEvent){}, b.messageHandlers...)
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
	b.messageHandlers = append(b.messageHandlers, handler)
	return nil
}

func (b *MemoryRealtimeBus) PublishPresenceChanged(event PresenceChangedEvent) error {
	b.mu.RLock()
	handlers := append([]func(PresenceChangedEvent){}, b.presenceHandlers...)
	b.mu.RUnlock()
	for _, handler := range handlers {
		handler(event)
	}
	return nil
}

func (b *MemoryRealtimeBus) SubscribePresenceChanged(handler func(PresenceChangedEvent)) error {
	if handler == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.presenceHandlers = append(b.presenceHandlers, handler)
	return nil
}

func (b *MemoryRealtimeBus) PublishConversationUnreadChanged(event ConversationUnreadChangedEvent) error {
	b.mu.RLock()
	handlers := append([]func(ConversationUnreadChangedEvent){}, b.conversationUnreadHandlers...)
	b.mu.RUnlock()
	for _, handler := range handlers {
		handler(event)
	}
	return nil
}

func (b *MemoryRealtimeBus) SubscribeConversationUnreadChanged(handler func(ConversationUnreadChangedEvent)) error {
	if handler == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.conversationUnreadHandlers = append(b.conversationUnreadHandlers, handler)
	return nil
}

func (b *MemoryRealtimeBus) PublishConversationChanged(event ConversationChangedEvent) error {
	b.mu.RLock()
	handlers := append([]func(ConversationChangedEvent){}, b.conversationChangedHandlers...)
	b.mu.RUnlock()
	for _, handler := range handlers {
		handler(event)
	}
	return nil
}

func (b *MemoryRealtimeBus) SubscribeConversationChanged(handler func(ConversationChangedEvent)) error {
	if handler == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.conversationChangedHandlers = append(b.conversationChangedHandlers, handler)
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

func (b *RedisRealtimeBus) PublishPresenceChanged(event PresenceChangedEvent) error {
	return b.publish(b.presenceChannel(), event)
}

func (b *RedisRealtimeBus) PublishConversationUnreadChanged(event ConversationUnreadChangedEvent) error {
	return b.publish(b.conversationUnreadChannel(), event)
}

func (b *RedisRealtimeBus) PublishConversationChanged(event ConversationChangedEvent) error {
	return b.publish(b.conversationChangedChannel(), event)
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

func (b *RedisRealtimeBus) SubscribePresenceChanged(handler func(PresenceChangedEvent)) error {
	if handler == nil {
		return nil
	}
	return b.subscribe(b.presenceChannel(), func(payload []byte) {
		var event PresenceChangedEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			log.Printf("解析 Redis 在线状态事件失败: %v", err)
			return
		}
		handler(event)
	})
}

func (b *RedisRealtimeBus) SubscribeConversationUnreadChanged(handler func(ConversationUnreadChangedEvent)) error {
	if handler == nil {
		return nil
	}
	return b.subscribe(b.conversationUnreadChannel(), func(payload []byte) {
		var event ConversationUnreadChangedEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			log.Printf("解析 Redis 会话未读事件失败: %v", err)
			return
		}
		handler(event)
	})
}

func (b *RedisRealtimeBus) SubscribeConversationChanged(handler func(ConversationChangedEvent)) error {
	if handler == nil {
		return nil
	}
	return b.subscribe(b.conversationChangedChannel(), func(payload []byte) {
		var event ConversationChangedEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			log.Printf("解析 Redis 会话变化事件失败: %v", err)
			return
		}
		handler(event)
	})
}

func (b *RedisRealtimeBus) publish(channel string, event any) error {
	if b == nil || b.client == nil {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return b.client.Publish(redisContext(), channel, payload).Err()
}

func (b *RedisRealtimeBus) subscribe(channel string, handler func([]byte)) error {
	if b == nil || b.client == nil || handler == nil {
		return nil
	}
	pubsub := b.client.Subscribe(redisContext(), channel)
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
			handler([]byte(message.Payload))
		}
	}()
	return nil
}

func (b *RedisRealtimeBus) presenceChannel() string {
	return b.channel + ":presence"
}

func (b *RedisRealtimeBus) conversationUnreadChannel() string {
	return b.channel + ":conversation_unread"
}

func (b *RedisRealtimeBus) conversationChangedChannel() string {
	return b.channel + ":conversation_changed"
}
