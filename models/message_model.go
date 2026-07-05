package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	// MessageTypeText 是普通文本消息。
	MessageTypeText = "text"
	// MessageTypeImage 是图片消息，v3 只保存图片 URL 和基础元数据。
	MessageTypeImage = "image"
	// MessageTypeFile 是文件消息，v3 只保存文件 URL 和基础元数据。
	MessageTypeFile = "file"
	// MessageTypeSystem 预留给服务端系统消息，普通 HTTP 发送接口不允许客户端提交。
	MessageTypeSystem = "system"

	// MessageStatusNormal 表示消息正常展示。
	MessageStatusNormal = "normal"
)

const (
	defaultMessagePageLimit = 20
	maxMessagePageLimit     = 100
)

var (
	// ErrMessageNotFound 表示消息不存在。
	ErrMessageNotFound = errors.New("消息不存在")
	// ErrInvalidMessageType 表示消息类型不在当前版本允许范围内。
	ErrInvalidMessageType = errors.New("无效消息类型")
	// ErrInvalidMessageContent 表示 content JSON 结构不符合消息类型要求。
	ErrInvalidMessageContent = errors.New("无效消息内容")
	// ErrMessageForbidden 表示当前用户不是会话 active 成员，不能操作消息。
	ErrMessageForbidden = errors.New("无权操作该消息")
	// ErrReadCursorInvalid 表示已读游标不属于当前会话。
	ErrReadCursorInvalid = errors.New("无效已读游标")
)

// Message 映射 messages 表。所有单聊和群聊消息统一存在这里。
type Message struct {
	ID             int64           `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ConversationID int64           `gorm:"column:conversation_id" json:"conversation_id"`
	SenderID       int64           `gorm:"column:sender_id" json:"sender_id"`
	ClientMsgID    string          `gorm:"column:client_msg_id" json:"client_msg_id"`
	MessageType    string          `gorm:"column:message_type" json:"message_type"`
	Content        json.RawMessage `gorm:"column:content" json:"content"`
	Status         string          `gorm:"column:status" json:"status"`
	SentAt         time.Time       `gorm:"column:sent_at" json:"sent_at"`
	CreatedAt      time.Time       `gorm:"column:created_at" json:"created_at"`
}

func (Message) TableName() string {
	return "messages"
}

// MessageDTO 是消息接口对外返回的结构。
// 当前版本只返回正常消息，状态固定为 normal。
type MessageDTO struct {
	ID             int64           `json:"id"`
	ConversationID int64           `json:"conversation_id"`
	SenderID       int64           `json:"sender_id"`
	ClientMsgID    string          `json:"client_msg_id,omitempty"`
	MessageType    string          `json:"message_type"`
	Content        json.RawMessage `json:"content"`
	Status         string          `json:"status"`
	SentAt         string          `json:"sent_at"`
}

// SendMessageResult 表示一次发送请求的结果。
// Created 用于区分真正新写入的消息和 client_msg_id 幂等命中的旧消息。
type SendMessageResult struct {
	Message MessageDTO
	Created bool
}

// MessagePageDTO 是历史消息分页响应。
// NextBeforeID 使用当前页最后一条消息 ID，客户端下一页传 before_id 即可继续向前翻。
type MessagePageDTO struct {
	Items        []MessageDTO `json:"items"`
	NextBeforeID int64        `json:"next_before_id"`
	HasMore      bool         `json:"has_more"`
}

// ReadStateDTO 是标记已读后的游标状态。
// last_read_message_id 只会向前推进，不会因为客户端传较小 ID 而回退。
type ReadStateDTO struct {
	ConversationID    int64  `json:"conversation_id"`
	LastReadMessageID int64  `json:"last_read_message_id"`
	LastReadAt        string `json:"last_read_at"`
}

// SendMessage 写入消息，并同步更新会话最后消息。
// 这个方法是 HTTP 发送和后续 WebSocket 发送的共同入口，避免两套入库逻辑分叉。
func SendMessage(senderID int64, conversationID int64, clientMsgID string, messageType string, content json.RawMessage) (MessageDTO, error) {
	result, err := SendMessageWithResult(senderID, conversationID, clientMsgID, messageType, content)
	if err != nil {
		return MessageDTO{}, err
	}
	return result.Message, nil
}

// SendMessageWithResult 写入消息，并返回该请求是否真正创建了新消息。
// HTTP 重试或 WebSocket ack 超时后的兜底发送可能命中同一个 client_msg_id；
// 这类幂等请求应返回已有消息，但不应再次触发实时投递。
func SendMessageWithResult(senderID int64, conversationID int64, clientMsgID string, messageType string, content json.RawMessage) (SendMessageResult, error) {
	clientMsgID = strings.TrimSpace(clientMsgID)
	messageType = strings.TrimSpace(messageType)
	if senderID <= 0 || conversationID <= 0 || clientMsgID == "" {
		return SendMessageResult{}, ErrValidation
	}
	if err := validateMessageContent(messageType, content); err != nil {
		return SendMessageResult{}, err
	}

	var saved Message
	created := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 发送消息前必须确认发送者仍是 active 成员。
		if _, err := ensureActiveMemberWithDB(tx, senderID, conversationID); err != nil {
			return err
		}

		// client_msg_id 用于客户端重试幂等；已有消息直接返回。
		existing, err := findMessageByClientMsgIDWithDB(tx, senderID, clientMsgID)
		if err == nil {
			// client_msg_id 只允许作为“同一发送者、同一会话”的重试凭据。
			// 如果客户端把同一个 client_msg_id 复用到另一个会话，应按参数错误处理，而不是返回旧会话消息。
			if existing.ConversationID != conversationID {
				return ErrValidation
			}
			saved = existing
			created = false
			return nil
		}
		if !errors.Is(err, ErrMessageNotFound) {
			return err
		}

		message := Message{
			ConversationID: conversationID,
			SenderID:       senderID,
			ClientMsgID:    clientMsgID,
			MessageType:    messageType,
			Content:        content,
			Status:         MessageStatusNormal,
			SentAt:         time.Now(),
		}
		if err := tx.Create(&message).Error; err != nil {
			if isDuplicateKey(err) {
				// 并发重试可能同时 INSERT 同一个 client_msg_id。
				// 其中一个请求成功后，其它请求命中唯一键冲突，转为查询已有消息并按幂等成功返回。
				existing, findErr := findMessageByClientMsgIDWithDB(tx, senderID, clientMsgID)
				if findErr != nil {
					return findErr
				}
				if existing.ConversationID != conversationID {
					return ErrValidation
				}
				saved = existing
				created = false
				return nil
			}
			return err
		}

		updates := map[string]any{
			"last_message_id": message.ID,
			"last_message_at": message.SentAt,
		}
		if err := tx.Model(&Conversation{}).Where("id = ?", conversationID).Updates(updates).Error; err != nil {
			return err
		}
		saved = message
		created = true
		return nil
	})
	if err != nil {
		return SendMessageResult{}, fmt.Errorf("发送消息失败: %w", err)
	}
	return SendMessageResult{
		Message: saved.ToDTO(),
		Created: created,
	}, nil
}

// ListMessages 按消息 ID 游标分页查询历史消息。
func ListMessages(userID int64, conversationID int64, beforeID int64, limit int) (MessagePageDTO, error) {
	if err := EnsureMessageAccess(userID, conversationID); err != nil {
		return MessagePageDTO{}, err
	}

	pageLimit := normalizeMessagePageLimit(limit)
	// 多查 1 条用于判断 has_more，不需要额外 COUNT。
	// 聊天历史通常只关心“是否还有上一页”，COUNT 大表成本更高。
	queryLimit := pageLimit + 1

	query := DB.Where("conversation_id = ? AND status = ?", conversationID, MessageStatusNormal)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}

	var messages []Message
	if err := query.Order("id desc").Limit(queryLimit).Find(&messages).Error; err != nil {
		return MessagePageDTO{}, fmt.Errorf("查询历史消息失败: %w", err)
	}

	hasMore := len(messages) > pageLimit
	if hasMore {
		messages = messages[:pageLimit]
	}

	// 数据库按 id desc 取最近消息，接口保持这个顺序返回；
	// 前端如需从旧到新展示，可以在本地按 id 升序排列。
	items := make([]MessageDTO, 0, len(messages))
	var nextBeforeID int64
	for _, message := range messages {
		items = append(items, message.ToDTO())
		nextBeforeID = message.ID
	}

	return MessagePageDTO{
		Items:        items,
		NextBeforeID: nextBeforeID,
		HasMore:      hasMore,
	}, nil
}

// MarkConversationRead 更新当前用户在会话中的已读游标。
func MarkConversationRead(userID int64, conversationID int64, lastReadMessageID int64) (ReadStateDTO, error) {
	if userID <= 0 || conversationID <= 0 || lastReadMessageID <= 0 {
		return ReadStateDTO{}, ErrValidation
	}

	var state ReadStateDTO
	err := DB.Transaction(func(tx *gorm.DB) error {
		member, err := ensureActiveMemberWithDB(tx, userID, conversationID)
		if err != nil {
			return err
		}

		message, err := findMessageByIDWithDB(tx, lastReadMessageID)
		if err != nil {
			return err
		}
		if message.ConversationID != conversationID {
			// 已读游标必须属于当前会话，不能用其它会话的消息 ID 推进游标。
			return ErrReadCursorInvalid
		}

		currentReadID := int64Value(member.LastReadMessageID)
		readAt := time.Now()
		if lastReadMessageID > currentReadID {
			updates := map[string]any{
				"last_read_message_id": lastReadMessageID,
				"last_read_at":         readAt,
			}
			err = tx.Model(&ConversationMember{}).
				Where("conversation_id = ? AND user_id = ?", conversationID, userID).
				Updates(updates).Error
			if err != nil {
				return err
			}
			state = ReadStateDTO{
				ConversationID:    conversationID,
				LastReadMessageID: lastReadMessageID,
				LastReadAt:        readAt.Format(time.RFC3339),
			}
			return nil
		}

		state = ReadStateDTO{
			ConversationID:    conversationID,
			LastReadMessageID: currentReadID,
			LastReadAt:        timeString(member.LastReadAt),
		}
		return nil
	})
	if err != nil {
		return ReadStateDTO{}, fmt.Errorf("标记已读失败: %w", err)
	}
	return state, nil
}

// FindMessageByID 根据消息 ID 查询消息。
func FindMessageByID(messageID int64) (Message, error) {
	return findMessageByIDWithDB(DB, messageID)
}

// EnsureMessageAccess 复用会话 active 成员校验，并把错误转换到消息领域。
func EnsureMessageAccess(userID int64, conversationID int64) error {
	if _, err := EnsureActiveMember(userID, conversationID); err != nil {
		if errors.Is(err, ErrConversationForbidden) {
			return ErrMessageForbidden
		}
		return err
	}
	return nil
}

// ToDTO 把消息数据库模型转换成接口输出模型。
func (m Message) ToDTO() MessageDTO {
	return MessageDTO{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		SenderID:       m.SenderID,
		ClientMsgID:    m.ClientMsgID,
		MessageType:    m.MessageType,
		Content:        m.Content,
		Status:         m.Status,
		SentAt:         m.SentAt.Format(time.RFC3339),
	}
}

// validateMessageContent 按消息类型校验 content JSON。
// 当前只允许客户端发送 text/image/file，system 留给服务端未来生成系统消息。
func validateMessageContent(messageType string, content json.RawMessage) error {
	if len(content) == 0 || !json.Valid(content) {
		return ErrInvalidMessageContent
	}

	switch messageType {
	case MessageTypeText:
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(content, &payload); err != nil {
			return ErrInvalidMessageContent
		}
		if strings.TrimSpace(payload.Text) == "" {
			return ErrInvalidMessageContent
		}
		return nil
	case MessageTypeImage:
		var payload struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(content, &payload); err != nil {
			return ErrInvalidMessageContent
		}
		if strings.TrimSpace(payload.URL) == "" {
			return ErrInvalidMessageContent
		}
		return nil
	case MessageTypeFile:
		var payload struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(content, &payload); err != nil {
			return ErrInvalidMessageContent
		}
		if strings.TrimSpace(payload.URL) == "" {
			return ErrInvalidMessageContent
		}
		return nil
	case MessageTypeSystem:
		return ErrInvalidMessageType
	default:
		return ErrInvalidMessageType
	}
}

// normalizeMessagePageLimit 把分页 limit 归一到安全范围。
// 未传或传 0 使用默认值，超过上限时截断，避免一次查询过多历史消息。
func normalizeMessagePageLimit(limit int) int {
	if limit <= 0 {
		return defaultMessagePageLimit
	}
	if limit > maxMessagePageLimit {
		return maxMessagePageLimit
	}
	return limit
}

// findMessageByIDWithDB 在指定事务/连接中按主键查消息。
// 传入 db 参数可以让发送、已读等事务复用同一个查询 helper。
func findMessageByIDWithDB(db *gorm.DB, messageID int64) (Message, error) {
	if messageID <= 0 {
		return Message{}, ErrMessageNotFound
	}
	var message Message
	err := db.First(&message, messageID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Message{}, ErrMessageNotFound
	}
	if err != nil {
		return Message{}, fmt.Errorf("查询消息失败: %w", err)
	}
	return message, nil
}

// findMessageByClientMsgIDWithDB 根据发送者和 client_msg_id 查找幂等消息。
// client_msg_id 的唯一性只在同一个发送者范围内成立，不要求全局唯一。
func findMessageByClientMsgIDWithDB(db *gorm.DB, senderID int64, clientMsgID string) (Message, error) {
	var message Message
	result := db.Where("sender_id = ? AND client_msg_id = ?", senderID, clientMsgID).Limit(1).Find(&message)
	if result.Error != nil {
		return Message{}, fmt.Errorf("查询消息失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return Message{}, ErrMessageNotFound
	}
	return message, nil
}

// ensureActiveMemberWithDB 在消息领域校验 active 成员身份。
// 与会话 helper 不同，这里会把无权限映射成 ErrMessageForbidden，方便 controller 返回消息领域文案。
func ensureActiveMemberWithDB(db *gorm.DB, userID int64, conversationID int64) (ConversationMember, error) {
	if userID <= 0 || conversationID <= 0 {
		return ConversationMember{}, ErrInvalidMember
	}
	var member ConversationMember
	err := db.Where("conversation_id = ? AND user_id = ? AND status = ?", conversationID, userID, MemberStatusActive).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ConversationMember{}, ErrMessageForbidden
	}
	if err != nil {
		return ConversationMember{}, fmt.Errorf("查询会话成员失败: %w", err)
	}
	return member, nil
}
