package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// MessageReceiptController 处理逐条消息回执。
// 会话级已读仍由 /api/conversations/:id/read 维护；这里负责 v7 的“单条消息送达/已读详情”。
type MessageReceiptController struct{}

// Index 查询某条消息的回执列表。
// model 层会先校验当前用户是消息所在会话成员，避免非成员窥探回执状态。
func (ctl MessageReceiptController) Index(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}
	messageID, err := parseIDParam(c, "message_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	receipts, err := models.ListMessageReceipts(user.ID, messageID)
	if err != nil {
		writeReceiptError(c, err)
		return
	}
	responses.Success(c, receipts, "获取消息回执成功")
}

// Delivered 标记当前用户已经收到某条消息。
// WebSocket 实时投递成功后服务端也会尝试写 delivered；HTTP 接口用于客户端主动补偿。
func (ctl MessageReceiptController) Delivered(c *gin.Context) {
	ctl.mark(c, models.MessageReceiptDelivered, "标记送达成功")
}

// Read 标记当前用户已经读到某条消息。
// read 包含 delivered 语义，model 层会保证 delivered 不会覆盖已有 read。
func (ctl MessageReceiptController) Read(c *gin.Context) {
	ctl.mark(c, models.MessageReceiptRead, "标记已读成功")
}

func (ctl MessageReceiptController) mark(c *gin.Context, status string, message string) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}
	messageID, err := parseIDParam(c, "message_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	// UpsertMessageReceipt 会校验消息存在、当前用户是会话成员，并处理 delivered/read 的状态升级。
	if err := models.UpsertMessageReceipt(messageID, user.ID, status); err != nil {
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
