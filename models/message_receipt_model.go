package models

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MessageReceiptDelivered = "delivered"
	MessageReceiptRead      = "read"
)

var ErrInvalidReceiptStatus = errors.New("无效回执状态")

// MessageReceipt 映射 message_receipts 表。
// 一条消息对一个用户最多一条回执，status 从 delivered 升级到 read。
type MessageReceipt struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	MessageID int64     `gorm:"column:message_id" json:"message_id"`
	UserID    int64     `gorm:"column:user_id" json:"user_id"`
	Status    string    `gorm:"column:status" json:"status"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (MessageReceipt) TableName() string {
	return "message_receipts"
}

type MessageReceiptDTO struct {
	MessageID int64  `json:"message_id"`
	UserID    int64  `json:"user_id"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

func (r MessageReceipt) ToDTO() MessageReceiptDTO {
	return MessageReceiptDTO{
		MessageID: r.MessageID,
		UserID:    r.UserID,
		Status:    r.Status,
		UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
	}
}

func UpsertMessageReceipt(messageID int64, userID int64, status string) error {
	if status != MessageReceiptDelivered && status != MessageReceiptRead {
		return ErrInvalidReceiptStatus
	}

	message, err := FindMessageByID(messageID)
	if err != nil {
		return err
	}
	if err := EnsureMessageAccess(userID, message.ConversationID); err != nil {
		return err
	}

	now := time.Now()
	receipt := MessageReceipt{
		MessageID: messageID,
		UserID:    userID,
		Status:    status,
		UpdatedAt: now,
	}

	assignments := map[string]any{
		"updated_at": now,
	}
	if status == MessageReceiptDelivered {
		// read 包含 delivered 语义；如果已有 read，后续 delivered 不能降级。
		assignments["status"] = gorm.Expr("IF(status = ?, status, ?)", MessageReceiptRead, MessageReceiptDelivered)
	} else {
		assignments["status"] = status
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

func MarkMessageDelivered(messageID int64, userID int64) error {
	return UpsertMessageReceipt(messageID, userID, MessageReceiptDelivered)
}

func MarkMessageRead(messageID int64, userID int64) error {
	return UpsertMessageReceipt(messageID, userID, MessageReceiptRead)
}

func ListMessageReceipts(requestUserID int64, messageID int64) ([]MessageReceiptDTO, error) {
	message, err := FindMessageByID(messageID)
	if err != nil {
		return nil, err
	}
	if err := EnsureMessageAccess(requestUserID, message.ConversationID); err != nil {
		return nil, err
	}

	var receipts []MessageReceipt
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
