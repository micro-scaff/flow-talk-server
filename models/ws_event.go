package models

import (
	"encoding/json"
	"time"
)

const (
	// WSEventPing / WSEventPong 是应用层心跳事件。
	// WebSocket 协议自身也有 ping frame，但前端用 JSON 心跳更容易调试。
	WSEventPing = "ping"
	WSEventPong = "pong"

	// WSEventMessageSend 是客户端通过长连接发送消息的事件名。
	WSEventMessageSend = "message.send"
	// WSEventMessageAck 表示服务端已经接收并完成消息入库。
	WSEventMessageAck = "message.ack"
	// WSEventMessageDeliver 表示服务端向在线成员实时投递消息。
	WSEventMessageDeliver = "message.deliver"
	// WSEventPresenceChanged 表示某个用户的在线连接状态发生变化。
	WSEventPresenceChanged = "presence.changed"
	// WSEventConversationUnreadChanged 表示当前用户某个会话的权威未读状态发生变化。
	WSEventConversationUnreadChanged = "conversation.unread.changed"
	// WSEventConversationChanged 表示会话资料、成员或权限发生变化，客户端应刷新会话快照。
	WSEventConversationChanged = "conversation.changed"
	// WSEventError 表示某个 WebSocket 请求事件处理失败。
	WSEventError = "error"

	// ConversationChangeMembers 覆盖成员增删、退出及角色变化。
	ConversationChangeMembers = "members"
	// ConversationChangeProfile 表示群名称或头像变化。
	ConversationChangeProfile = "profile"
)

// WSEvent 是客户端和服务端之间统一使用的 JSON 事件信封。
// RequestID 由客户端传入，服务端在 ack/error/pong 中原样带回，方便客户端匹配请求。
type WSEvent struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// WSMessageSendPayload 是 message.send 的 payload。
// 字段和 HTTP 发送消息接口保持一致，保证两条入口最终复用同一个 SendMessage 业务方法。
type WSMessageSendPayload struct {
	ConversationID int64           `json:"conversation_id"`
	ClientMsgID    string          `json:"client_msg_id"`
	MessageType    string          `json:"message_type"`
	Content        json.RawMessage `json:"content"`
}

// WSPongPayload 是 pong 事件返回给客户端的最小服务端状态。
type WSPongPayload struct {
	ServerTime string `json:"server_time"`
}

// WSErrorPayload 是 WebSocket error 事件的 payload。
// Message 面向客户端展示，内部错误细节不直接透出。
type WSErrorPayload struct {
	Message string `json:"message"`
}

// ConversationChangedPayload 告诉客户端需要重新拉取哪个会话以及变化类别。
// 事件不携带局部快照，避免多实例或连续变更时客户端合并出过期状态。
type ConversationChangedPayload struct {
	ConversationID int64  `json:"conversation_id"`
	ChangeType     string `json:"change_type"`
}

// NewWSEvent 负责把任意 payload 包装成统一事件。
// 这里集中做 JSON 序列化，controller 只关心业务含义，不关心信封细节。
func NewWSEvent(eventType string, requestID string, payload any) WSEvent {
	var raw json.RawMessage
	if payload != nil {
		if encoded, err := json.Marshal(payload); err == nil {
			raw = encoded
		}
	}
	return WSEvent{
		Type:      eventType,
		RequestID: requestID,
		Payload:   raw,
	}
}

// NewPongEvent 创建心跳响应事件。
func NewPongEvent(requestID string) WSEvent {
	return NewWSEvent(WSEventPong, requestID, WSPongPayload{
		ServerTime: time.Now().Format(time.RFC3339),
	})
}

// NewWSErrorEvent 创建标准错误事件。
func NewWSErrorEvent(requestID string, message string) WSEvent {
	return NewWSEvent(WSEventError, requestID, WSErrorPayload{Message: message})
}
