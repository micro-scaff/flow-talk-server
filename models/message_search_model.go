package models

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 100
)

// SearchConversationMessages 搜索指定会话内的文本消息。
func SearchConversationMessages(userID int64, conversationID int64, keyword string, limit int) ([]MessageDTO, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, ErrValidation
	}
	if err := EnsureMessageAccess(userID, conversationID); err != nil {
		return nil, err
	}

	return searchMessages(DB.Where("conversation_id = ?", conversationID), keyword, limit)
}

// SearchMyMessages 搜索当前用户参与的所有会话文本消息。
func SearchMyMessages(userID int64, keyword string, limit int) ([]MessageDTO, error) {
	keyword = strings.TrimSpace(keyword)
	if userID <= 0 || keyword == "" {
		return nil, ErrValidation
	}

	query := DB.Table("messages").
		Joins("JOIN conversation_members cm ON cm.conversation_id = messages.conversation_id").
		Where("cm.user_id = ? AND cm.status = ?", userID, MemberStatusActive)
	return searchMessages(query, keyword, limit)
}

func searchMessages(query *gorm.DB, keyword string, limit int) ([]MessageDTO, error) {
	pageLimit := normalizeSearchLimit(limit)
	like := "%" + keyword + "%"

	var messages []Message
	err := query.
		Where("messages.message_type = ?", MessageTypeText).
		Where("messages.status = ?", MessageStatusNormal).
		Where("JSON_UNQUOTE(JSON_EXTRACT(messages.content, '$.text')) LIKE ?", like).
		Order("messages.id desc").
		Limit(pageLimit).
		Find(&messages).Error
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}

	result := make([]MessageDTO, 0, len(messages))
	for _, message := range messages {
		result = append(result, message.ToDTO())
	}
	return result, nil
}

func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultSearchLimit
	}
	if limit > maxSearchLimit {
		return maxSearchLimit
	}
	return limit
}
