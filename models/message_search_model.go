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
	// keyword 为空时直接拒绝，避免一次请求扫完整张消息表。
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, ErrValidation
	}
	// 会话内搜索必须先校验 membership。
	// 搜索和拉历史消息一样，都属于读取会话内容的权限范围。
	if err := EnsureMessageAccess(userID, conversationID); err != nil {
		return nil, err
	}

	return searchMessages(DB.Where("conversation_id = ?", conversationID), keyword, limit)
}

// SearchMyMessages 搜索当前用户参与的所有会话文本消息。
func SearchMyMessages(userID int64, keyword string, limit int) ([]MessageDTO, error) {
	// 全局搜索也要求 keyword 非空，避免误触发大范围 LIKE 查询。
	keyword = strings.TrimSpace(keyword)
	if userID <= 0 || keyword == "" {
		return nil, ErrValidation
	}

	// 通过 conversation_members 限定当前用户 active 会话。
	// 这样即使 messages 表里有其它会话内容，也不会被全局搜索泄露。
	query := DB.Table("messages").
		Joins("JOIN conversation_members cm ON cm.conversation_id = messages.conversation_id").
		Where("cm.user_id = ? AND cm.status = ?", userID, MemberStatusActive)
	return searchMessages(query, keyword, limit)
}

func searchMessages(query *gorm.DB, keyword string, limit int) ([]MessageDTO, error) {
	pageLimit := normalizeSearchLimit(limit)
	// 参数仍然通过占位符传给 MySQL，不拼接 SQL，避免 LIKE 查询中的注入风险。
	like := "%" + keyword + "%"

	var messages []Message
	err := query.
		// 初期只搜索文本消息，图片/文件的 URL 或文件名后续可以单独设计搜索字段。
		Where("messages.message_type = ?", MessageTypeText).
		Where("messages.status = ?", MessageStatusNormal).
		// content 是 JSON 字段，文本消息约定 content.text 保存正文。
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
	// 给 limit 设置上限，避免一个搜索请求返回过多数据拖慢接口。
	if limit <= 0 {
		return defaultSearchLimit
	}
	if limit > maxSearchLimit {
		return maxSearchLimit
	}
	return limit
}
