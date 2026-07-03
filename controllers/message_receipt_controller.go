package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// MessageReceiptController 处理逐条消息回执。
// 会话级已读仍由 /api/conversations/read 维护；这里负责 v7 的“单条消息已读/未读详情”。
type MessageReceiptController struct{}

// MessageReceiptRequest 是逐条消息回执请求。
type MessageReceiptRequest struct {
	MessageID int64 `json:"message_id" binding:"required"`
}

// Index 查询某条消息的回执列表。
// model 层会先校验当前用户是消息所在会话成员，避免非成员窥探回执状态。
func (ctl MessageReceiptController) Index(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	var req MessageReceiptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	receipts, err := models.ListMessageReceipts(user.ID, req.MessageID)
	if err != nil {
		writeReceiptError(c, err)
		return
	}
	responses.Success(c, receipts, "获取消息回执成功")
}

// Read 标记当前用户已经读到某条消息。
func (ctl MessageReceiptController) Read(c *gin.Context) {
	ctl.mark(c, models.MessageReceiptRead, "标记已读成功")
}

// Unread 标记当前用户需要把某条消息重新作为未读处理。
func (ctl MessageReceiptController) Unread(c *gin.Context) {
	ctl.mark(c, models.MessageReceiptUnread, "标记未读成功")
}

func (ctl MessageReceiptController) mark(c *gin.Context, status string, message string) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	var req MessageReceiptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	// UpsertMessageReceipt 会校验消息存在、当前用户是会话成员，并写入 read/unread 状态。
	if err := models.UpsertMessageReceipt(req.MessageID, user.ID, status); err != nil {
		writeReceiptError(c, err)
		return
	}
	responses.Success(c, nil, message)
}

func writeReceiptError(c *gin.Context, err error) {
	// 回执接口的权限以消息所在会话为准。
	// 这里把消息领域错误转换成稳定 HTTP 响应，避免 controller 方法重复 switch。
	switch {
	case errors.Is(err, models.ErrInvalidReceiptStatus),
		errors.Is(err, models.ErrInvalidMember):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	case errors.Is(err, models.ErrMessageForbidden),
		errors.Is(err, models.ErrConversationForbidden):
		responses.Error(c, http.StatusForbidden, "无权操作该消息")
	case errors.Is(err, models.ErrMessageNotFound):
		responses.Error(c, http.StatusNotFound, "消息不存在")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
