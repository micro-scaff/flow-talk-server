package controllers

import (
	"encoding/json"
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// MessageController 处理会话内消息相关接口。
type MessageController struct {
	Hub *models.WSHub
	Bus models.RealtimeBus
}

// SendMessageRequest 是发送消息的请求体。
// Content 使用 json.RawMessage，让 model 层按 message_type 做结构化校验。
type SendMessageRequest struct {
	ConversationID int64           `json:"conversation_id" binding:"required"`
	ClientMsgID    string          `json:"client_msg_id" binding:"required"`
	MessageType    string          `json:"message_type" binding:"required"`
	Content        json.RawMessage `json:"content" binding:"required"`
}

// ListMessagesRequest 是分页拉取历史消息的请求体。
type ListMessagesRequest struct {
	ConversationID int64 `json:"conversation_id" binding:"required"`
	BeforeID       int64 `json:"before_id"`
	Limit          int   `json:"limit"`
}

// MarkReadRequest 是标记会话已读的请求体。
type MarkReadRequest struct {
	ConversationID    int64 `json:"conversation_id" binding:"required"`
	LastReadMessageID int64 `json:"last_read_message_id" binding:"required"`
}

// Create 发送消息并写入 messages 表。
func (ctl MessageController) Create(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	result, err := models.SendMessageWithResult(user.ID, req.ConversationID, req.ClientMsgID, req.MessageType, req.Content)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	message := result.Message

	if result.Created {
		memberIDs, err := models.ListActiveConversationMemberIDs(message.ConversationID)
		if err != nil {
			writeMessageError(c, err)
			return
		}
		if err := publishCreatedMessageUpdates(ctl.Bus, ctl.Hub, memberIDs, message); err != nil {
			writeMessageError(c, err)
			return
		}
	}

	responses.Success(c, message, "发送消息成功")
}

// Index 分页拉取会话历史消息。
func (ctl MessageController) Index(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	var req ListMessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	page, err := models.ListMessages(user.ID, req.ConversationID, req.BeforeID, req.Limit)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	responses.Success(c, page, "获取历史消息成功")
}

// MarkRead 更新当前用户在会话内的已读游标。
func (ctl MessageController) MarkRead(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	var req MarkReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	state, err := models.MarkConversationRead(user.ID, req.ConversationID, req.LastReadMessageID)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	if err := publishConversationUnreadChanged(ctl.Bus, ctl.Hub, models.ConversationUnreadChangedEvent{
		UserID: user.ID,
		State:  state,
	}); err != nil {
		writeMessageError(c, err)
		return
	}
	responses.Success(c, state, "标记已读成功")
}

// writeMessageError 把消息领域错误翻译成 HTTP 状态码。
func writeMessageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, models.ErrValidation),
		errors.Is(err, models.ErrInvalidMember),
		errors.Is(err, models.ErrInvalidMessageType),
		errors.Is(err, models.ErrInvalidMessageContent),
		errors.Is(err, models.ErrReadCursorInvalid):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	case errors.Is(err, models.ErrMessageForbidden),
		errors.Is(err, models.ErrConversationForbidden):
		responses.Error(c, http.StatusForbidden, "无权操作该消息")
	case errors.Is(err, models.ErrMessageNotFound),
		errors.Is(err, models.ErrConversationNotFound):
		responses.Error(c, http.StatusNotFound, "消息或会话不存在")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
