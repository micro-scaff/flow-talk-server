package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// MessageSearchController 处理文本消息搜索接口。
type MessageSearchController struct{}

func (ctl MessageSearchController) SearchConversation(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}
	conversationID, err := parseIDParam(c, "conversation_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}
	limit, err := parseOptionalIntQuery(c, "limit")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	messages, err := models.SearchConversationMessages(user.ID, conversationID, c.Query("q"), limit)
	if err != nil {
		writeSearchError(c, err)
		return
	}
	responses.Success(c, messages, "搜索消息成功")
}

func (ctl MessageSearchController) SearchMine(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}
	limit, err := parseOptionalIntQuery(c, "limit")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	messages, err := models.SearchMyMessages(user.ID, c.Query("q"), limit)
	if err != nil {
		writeSearchError(c, err)
		return
	}
	responses.Success(c, messages, "搜索消息成功")
}

func writeSearchError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, models.ErrValidation),
		errors.Is(err, models.ErrInvalidMember):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	case errors.Is(err, models.ErrMessageForbidden),
		errors.Is(err, models.ErrConversationForbidden):
		responses.Error(c, http.StatusForbidden, "无权搜索该会话")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
