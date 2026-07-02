package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// MessageReceiptController 处理逐条消息回执。
type MessageReceiptController struct{}

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

func (ctl MessageReceiptController) Delivered(c *gin.Context) {
	ctl.mark(c, models.MessageReceiptDelivered, "标记送达成功")
}

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

	if err := models.UpsertMessageReceipt(messageID, user.ID, status); err != nil {
		writeReceiptError(c, err)
		return
	}
	responses.Success(c, nil, message)
}

func writeReceiptError(c *gin.Context, err error) {
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
