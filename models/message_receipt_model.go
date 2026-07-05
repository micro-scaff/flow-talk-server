package models

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm/clause"
)

const (
	// MessageReceiptUnread 表示当前用户把单条消息标记为未读。
	MessageReceiptUnread = "unread"
	// MessageReceiptRead 表示当前用户已经读过单条消息。
	MessageReceiptRead = "read"
)

// ErrInvalidReceiptStatus 表示回执状态不是 unread/read。
var ErrInvalidReceiptStatus = errors.New("无效回执状态")

// MessageReceipt 映射 message_receipts 表。
// 一条消息对一个用户最多一条回执，status 表示当前用户是否已读。
type MessageReceipt struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	MessageID int64     `gorm:"column:message_id" json:"message_id"`
	UserID    int64     `gorm:"column:user_id" json:"user_id"`
	Status    string    `gorm:"column:status" json:"status"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (MessageReceipt) TableName() string {
	// 明确表名，避免 GORM 把结构名推导成其它复数形式。
	return "message_receipts"
}

// MessageReceiptDTO 是回执接口返回给前端的结构。
// 不暴露自增 id，因为业务上唯一标识是 message_id + user_id。
type MessageReceiptDTO struct {
	MessageID int64  `json:"message_id"`
	UserID    int64  `json:"user_id"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

// ToDTO 把数据库回执转换成接口输出。
func (r MessageReceipt) ToDTO() MessageReceiptDTO {
	// updated_at 用 RFC3339 输出，和其它消息/会话时间字段保持一致。
	return MessageReceiptDTO{
		MessageID: r.MessageID,
		UserID:    r.UserID,
		Status:    r.Status,
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
}

func UpsertMessageReceipt(messageID int64, userID int64, status string) error {
	// 回执状态目前只有 read/unread 两种。
	// delivered 这类送达状态后续如果需要，应扩展常量和数据库约束后再开放。
	if status != MessageReceiptUnread && status != MessageReceiptRead {
		return ErrInvalidReceiptStatus
	}

	// 回执必须依附于真实消息。
	// 同时通过消息拿到 conversation_id，后面才能校验当前用户是否有权写这条回执。
	message, err := FindMessageByID(messageID)
	if err != nil {
		return err
	}
	// 只有消息所在会话的 active 成员才能写自己的回执。
	// 这可以防止任意用户给陌生消息伪造 read/unread 状态。
	if err := EnsureMessageAccess(userID, message.ConversationID); err != nil {
		return err
	}

	now := time.Now()
	// message_id + user_id 有唯一索引，天然适合 upsert。
	// 客户端重复上报 read/unread 时，只更新时间和当前状态。
	receipt := MessageReceipt{
		MessageID: messageID,
		UserID:    userID,
		Status:    status,
		UpdatedAt: now,
	}

	assignments := map[string]any{
		"updated_at": now,
		"status":     status,
	}

	err = DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(assignments),
	}).Create(&receipt).Error
	if err != nil {
		return fmt.Errorf("写入消息回执失败: %w", err)
	}
	return nil
}

func MarkMessageRead(messageID int64, userID int64) error {
	// 语义化包装，方便 controller 或后续内部调用不用关心具体字符串常量。
	return UpsertMessageReceipt(messageID, userID, MessageReceiptRead)
}

func MarkMessageUnread(messageID int64, userID int64) error {
	// 语义化包装，和 MarkMessageRead 保持对称。
	return UpsertMessageReceipt(messageID, userID, MessageReceiptUnread)
}

func ListMessageReceipts(requestUserID int64, messageID int64) ([]MessageReceiptDTO, error) {
	// 查询回执也要先确认请求者能看到这条消息。
	// 否则即使不返回消息正文，也会泄露群成员阅读状态。
	message, err := FindMessageByID(messageID)
	if err != nil {
		return nil, err
	}
	if err := EnsureMessageAccess(requestUserID, message.ConversationID); err != nil {
		return nil, err
	}

	var receipts []MessageReceipt
	// 按更新时间倒序，方便客户端优先展示最新发生的已读/送达变化。
	err = DB.Where("message_id = ?", messageID).Order("updated_at desc").Find(&receipts).Error
	if err != nil {
		return nil, fmt.Errorf("查询消息回执失败: %w", err)
	}

	result := make([]MessageReceiptDTO, 0, len(receipts))
	for _, receipt := range receipts {
		result = append(result, receipt.ToDTO())
	}
	return result, nil
}
