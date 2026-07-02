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
	// MessageStatusRecalled / MessageStatusDeleted 预留给 v6 消息状态能力。
	MessageStatusRecalled = "recalled"
	MessageStatusDeleted  = "deleted"
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
	// ErrInvalidMessageStatus 表示当前消息状态不允许继续执行该操作。
	ErrInvalidMessageStatus = errors.New("无效消息状态")
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

type MessagePageDTO struct {
	Items        []MessageDTO `json:"items"`
	NextBeforeID int64        `json:"next_before_id"`
	HasMore      bool         `json:"has_more"`
}

type ReadStateDTO struct {
	ConversationID    int64  `json:"conversation_id"`
	LastReadMessageID int64  `json:"last_read_message_id"`
	LastReadAt        string `json:"last_read_at"`
}

// SendMessage 写入消息，并同步更新会话最后消息。
// 这个方法是 HTTP 发送和后续 WebSocket 发送的共同入口，避免两套入库逻辑分叉。
func SendMessage(senderID int64, conversationID int64, clientMsgID string, messageType string, content json.RawMessage) (MessageDTO, error) {
	clientMsgID = strings.TrimSpace(clientMsgID)
	messageType = strings.TrimSpace(messageType)
	if senderID <= 0 || conversationID <= 0 || clientMsgID == "" {
		return MessageDTO{}, ErrValidation
	}
	if err := validateMessageContent(messageType, content); err != nil {
		return MessageDTO{}, err
	}

	var saved Message
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 发送消息前必须确认发送者仍是 active 成员。
		if _, err := ensureActiveMemberWithDB(tx, senderID, conversationID); err != nil {
			return err
		}

		// client_msg_id 用于客户端重试幂等；已有消息直接返回。
		existing, err := findMessageByClientMsgIDWithDB(tx, senderID, clientMsgID)
		if err == nil {
			saved = existing
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
				existing, findErr := findMessageByClientMsgIDWithDB(tx, senderID, clientMsgID)
				if findErr != nil {
					return findErr
				}
				saved = existing
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
		return nil
	})
	if err != nil {
		return MessageDTO{}, fmt.Errorf("发送消息失败: %w", err)
	}
	return saved.ToDTO(), nil
}

// ListMessages 按消息 ID 游标分页查询历史消息。
func ListMessages(userID int64, conversationID int64, beforeID int64, limit int) (MessagePageDTO, error) {
	if err := EnsureMessageAccess(userID, conversationID); err != nil {
		return MessagePageDTO{}, err
	}

	pageLimit := normalizeMessagePageLimit(limit)
	queryLimit := pageLimit + 1

	query := DB.Where("conversation_id = ? AND status <> ?", conversationID, MessageStatusDeleted)
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

// RecallMessage 撤回消息。
// 发送者可以撤回自己的消息；群主和管理员可以撤回群内其他成员消息。
func RecallMessage(operatorID int64, messageID int64) (MessageDTO, error) {
	return updateMessageStatus(operatorID, messageID, MessageStatusRecalled)
}

// DeleteMessage 删除消息。
// 当前版本的 deleted 是全局状态：被删除后不会再出现在历史消息列表中。
func DeleteMessage(operatorID int64, messageID int64) (MessageDTO, error) {
	return updateMessageStatus(operatorID, messageID, MessageStatusDeleted)
}

// CanManageMessage 判断用户是否可以管理某条消息。
// 这里不区分撤回和删除的权限矩阵，二者都遵循 v6 文档中的同一套规则。
func CanManageMessage(operatorID int64, message Message) (bool, error) {
	member, err := EnsureActiveMember(operatorID, message.ConversationID)
	if err != nil {
		if errors.Is(err, ErrConversationForbidden) {
			return false, ErrMessageForbidden
		}
		return false, err
	}
	if message.SenderID == operatorID {
		return true, nil
	}
	if member.Role == MemberRoleOwner || member.Role == MemberRoleAdmin {
		return true, nil
	}
	return false, nil
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

func updateMessageStatus(operatorID int64, messageID int64, status string) (MessageDTO, error) {
	if status != MessageStatusRecalled && status != MessageStatusDeleted {
		return MessageDTO{}, ErrInvalidMessageStatus
	}

	var saved Message
	err := DB.Transaction(func(tx *gorm.DB) error {
		message, err := findMessageByIDWithDB(tx, messageID)
		if err != nil {
			return err
		}
		if message.Status == MessageStatusDeleted {
			return ErrInvalidMessageStatus
		}

		canManage, err := CanManageMessage(operatorID, message)
		if err != nil {
			return err
		}
		if !canManage {
			return ErrMessageForbidden
		}

		if err := tx.Model(&message).Update("status", status).Error; err != nil {
			return err
		}
		message.Status = status
		saved = message
		return nil
	})
	if err != nil {
		return MessageDTO{}, fmt.Errorf("更新消息状态失败: %w", err)
	}
	return saved.ToDTO(), nil
}

func (m Message) ToDTO() MessageDTO {
	content := m.Content
	if m.Status == MessageStatusRecalled {
		content = json.RawMessage(`{}`)
	}
	return MessageDTO{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		SenderID:       m.SenderID,
		ClientMsgID:    m.ClientMsgID,
		MessageType:    m.MessageType,
		Content:        content,
		Status:         m.Status,
		SentAt:         m.SentAt.Format(time.RFC3339),
	}
}

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

func normalizeMessagePageLimit(limit int) int {
	if limit <= 0 {
		return defaultMessagePageLimit
	}
	if limit > maxMessagePageLimit {
		return maxMessagePageLimit
	}
	return limit
}

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

func findMessageByClientMsgIDWithDB(db *gorm.DB, senderID int64, clientMsgID string) (Message, error) {
	var message Message
	err := db.Where("sender_id = ? AND client_msg_id = ?", senderID, clientMsgID).First(&message).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Message{}, ErrMessageNotFound
	}
	if err != nil {
		return Message{}, fmt.Errorf("查询消息失败: %w", err)
	}
	return message, nil
}

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
