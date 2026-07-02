package models

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"
)

const (
	// wsSendBufferSize 是每个连接的服务端发送队列长度。
	// 队列满说明客户端消费太慢，当前版本选择丢弃本次实时投递，历史消息仍可通过 HTTP 拉取补齐。
	wsSendBufferSize = 32
)

// WSConnection 表示一个用户的一条 WebSocket 连接。
// 一个用户可以同时在 Web、移动端、桌面端建立多条连接，所以 Hub 不能只按 user_id 存一个连接。
type WSConnection struct {
	ID           string
	UserID       int64
	DeviceID     string
	ConnectedAt  time.Time
	LastActiveAt time.Time
	Send         chan []byte

	closeOnce sync.Once
}

// NewWSConnection 创建一条连接的内存对象。
func NewWSConnection(userID int64, deviceID string) *WSConnection {
	now := time.Now()
	return &WSConnection{
		ID:           strconv.FormatInt(userID, 10) + "-" + strconv.FormatInt(now.UnixNano(), 10),
		UserID:       userID,
		DeviceID:     deviceID,
		ConnectedAt:  now,
		LastActiveAt: now,
		Send:         make(chan []byte, wsSendBufferSize),
	}
}

// Close 关闭发送队列。closeOnce 防止断线、移除、异常路径重复 close channel。
func (c *WSConnection) Close() {
	c.closeOnce.Do(func() {
		close(c.Send)
	})
}

// SendEvent 把事件序列化后放入连接发送队列。
// 为了保护 Hub，不让一个慢客户端阻塞整个服务，队列满时直接放弃实时投递。
func (c *WSConnection) SendEvent(event WSEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	c.SendBytes(payload)
}

// SendBytes 使用 recover 兜底 channel 已关闭的极端竞态。
// Remove 和 Broadcast 可能在不同 goroutine 同时发生，这里保证投递失败不会拖垮服务。
func (c *WSConnection) SendBytes(payload []byte) {
	defer func() {
		_ = recover()
	}()
	select {
	case c.Send <- payload:
	default:
	}
}

// WSHub 是单进程内存连接管理器。
// v4 只要求单实例投递；v7 的多实例扩展会通过 RealtimeBus 接口在这个边界外继续扩展。
type WSHub struct {
	mu          sync.RWMutex
	connections map[int64]map[string]*WSConnection
}

func NewWSHub() *WSHub {
	return &WSHub{
		connections: make(map[int64]map[string]*WSConnection),
	}
}

// Add 注册连接。按 user_id 再按 connection_id 分组，便于给某个用户所有端投递。
func (h *WSHub) Add(conn *WSConnection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.connections[conn.UserID] == nil {
		h.connections[conn.UserID] = make(map[string]*WSConnection)
	}
	h.connections[conn.UserID][conn.ID] = conn
}

// Remove 移除连接并关闭发送队列。
func (h *WSHub) Remove(userID int64, connectionID string) {
	h.mu.Lock()
	conn := h.connections[userID][connectionID]
	delete(h.connections[userID], connectionID)
	if len(h.connections[userID]) == 0 {
		delete(h.connections, userID)
	}
	h.mu.Unlock()

	if conn != nil {
		conn.Close()
	}
}

// UserConnections 返回用户当前在本进程上的所有连接快照。
func (h *WSHub) UserConnections(userID int64) []*WSConnection {
	h.mu.RLock()
	defer h.mu.RUnlock()

	userConnections := h.connections[userID]
	result := make([]*WSConnection, 0, len(userConnections))
	for _, conn := range userConnections {
		result = append(result, conn)
	}
	return result
}

// BroadcastToUsers 给多个用户的所有本机连接投递同一份消息。
func (h *WSHub) BroadcastToUsers(userIDs []int64, payload []byte) {
	for _, conn := range h.connectionsForUsers(userIDs) {
		conn.SendBytes(payload)
	}
}

// BroadcastEventToUsers 是 BroadcastToUsers 的事件版本。
func (h *WSHub) BroadcastEventToUsers(userIDs []int64, event WSEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.BroadcastToUsers(userIDs, payload)
}

// Touch 更新连接活跃时间，供 ping 或任意客户端事件调用。
func (h *WSHub) Touch(userID int64, connectionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conn := h.connections[userID][connectionID]; conn != nil {
		conn.LastActiveAt = time.Now()
	}
}

// OnlineUserIDs 过滤出在本进程至少有一条连接的用户 ID。
// v7 回执写入 delivered 时会用它避免把离线用户误标记为已送达。
func (h *WSHub) OnlineUserIDs(userIDs []int64) []int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]int64, 0, len(userIDs))
	for _, userID := range userIDs {
		if len(h.connections[userID]) > 0 {
			result = append(result, userID)
		}
	}
	return result
}

// LocalPresence 返回本机视角下用户是否在线、连接数和最近活跃时间。
func (h *WSHub) LocalPresence(userID int64) PresenceDTO {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var lastActive time.Time
	connectionCount := 0
	for _, conn := range h.connections[userID] {
		connectionCount++
		if conn.LastActiveAt.After(lastActive) {
			lastActive = conn.LastActiveAt
		}
	}

	return PresenceDTO{
		UserID:          userID,
		Online:          connectionCount > 0,
		ConnectionCount: connectionCount,
		LastActiveAt:    formatOptionalTime(lastActive),
	}
}

func (h *WSHub) connectionsForUsers(userIDs []int64) []*WSConnection {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]*WSConnection, 0)
	for _, userID := range userIDs {
		for _, conn := range h.connections[userID] {
			result = append(result, conn)
		}
	}
	return result
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
