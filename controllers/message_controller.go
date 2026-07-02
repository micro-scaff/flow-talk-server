package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// MessageController 处理会话内消息相关接口。
type MessageController struct{}

// SendMessageRequest 是发送消息的请求体。
// Content 使用 json.RawMessage，让 model 层按 message_type 做结构化校验。
type SendMessageRequest struct {
	ClientMsgID string          `json:"client_msg_id" binding:"required"`
	MessageType string          `json:"message_type" binding:"required"`
	Content     json.RawMessage `json:"content" binding:"required"`
}

// MarkReadRequest 是标记会话已读的请求体。
type MarkReadRequest struct {
	LastReadMessageID int64 `json:"last_read_message_id" binding:"required"`
}

// Create 发送消息并写入 messages 表。
func (ctl MessageController) Create(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	conversationID, err := parseIDParam(c, "conversation_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	message, err := models.SendMessage(user.ID, conversationID, req.ClientMsgID, req.MessageType, req.Content)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	responses.Success(c, message, "发送消息成功")
}

// Index 分页拉取会话历史消息。
func (ctl MessageController) Index(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	conversationID, err := parseIDParam(c, "conversation_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	beforeID, err := parseOptionalInt64Query(c, "before_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}
	limit, err := parseOptionalIntQuery(c, "limit")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	page, err := models.ListMessages(user.ID, conversationID, beforeID, limit)
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

	conversationID, err := parseIDParam(c, "conversation_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	var req MarkReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	state, err := models.MarkConversationRead(user.ID, conversationID, req.LastReadMessageID)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	responses.Success(c, state, "标记已读成功")
}

// Recall 撤回一条消息。
func (ctl MessageController) Recall(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	messageID, err := parseIDParam(c, "message_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	message, err := models.RecallMessage(user.ID, messageID)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	responses.Success(c, message, "撤回消息成功")
}

// Delete 删除一条消息。
func (ctl MessageController) Delete(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	messageID, err := parseIDParam(c, "message_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	message, err := models.DeleteMessage(user.ID, messageID)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	responses.Success(c, message, "删除消息成功")
}

func parseOptionalInt64Query(c *gin.Context, name string) (int64, error) {
	value := c.Query(name)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid int64 query")
	}
	return parsed, nil
}

func parseOptionalIntQuery(c *gin.Context, name string) (int, error) {
	value := c.Query(name)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid int query")
	}
	return parsed, nil
}

// writeMessageError 把消息领域错误翻译成 HTTP 状态码。
func writeMessageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, models.ErrValidation),
		errors.Is(err, models.ErrInvalidMember),
		errors.Is(err, models.ErrInvalidMessageType),
		errors.Is(err, models.ErrInvalidMessageContent),
		errors.Is(err, models.ErrReadCursorInvalid),
		errors.Is(err, models.ErrInvalidMessageStatus):
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
