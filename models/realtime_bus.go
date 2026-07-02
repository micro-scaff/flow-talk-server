package models

import "sync"

// MessageDeliverEvent 是跨实例投递时传递的最小事件。
// 当前单机版直接通过 Hub 投递；这个结构为后续 Redis Pub/Sub 保留稳定边界。
type MessageDeliverEvent struct {
	UserIDs []int64    `json:"user_ids"`
	Message MessageDTO `json:"message"`
}

// RealtimeBus 抽象多实例之间的实时事件总线。
// 后续接 Redis 时，只需要实现这个接口，不需要改 HTTP/WebSocket 的业务入口。
type RealtimeBus interface {
	PublishMessageDeliver(event MessageDeliverEvent) error
	SubscribeMessageDeliver(handler func(MessageDeliverEvent)) error
}

// MemoryRealtimeBus 是单进程内存实现，主要用于开发和当前 v7 落地占位。
type MemoryRealtimeBus struct {
	mu       sync.RWMutex
	handlers []func(MessageDeliverEvent)
}

func NewMemoryRealtimeBus() *MemoryRealtimeBus {
	return &MemoryRealtimeBus{}
}

func (b *MemoryRealtimeBus) PublishMessageDeliver(event MessageDeliverEvent) error {
	// 先复制 handler 列表再调用，避免 handler 内部再次订阅时造成锁重入或长时间持锁。
	b.mu.RLock()
	handlers := append([]func(MessageDeliverEvent){}, b.handlers...)
	b.mu.RUnlock()

	// 内存实现同步执行 handler，便于本地调试。
	// Redis 实现可以改成异步消费，但对外仍保持同一个接口。
	for _, handler := range handlers {
		handler(event)
	}
	return nil
}

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
