package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// MessageSearchController 处理文本消息搜索接口。
// 当前版本只做 MySQL 范围内的文本消息搜索，不引入独立搜索引擎。
type MessageSearchController struct{}

// SearchConversation 搜索指定会话内的文本消息。
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

	// q 和 limit 都由 model 层做最终校验和归一化。
	// 会话内搜索必须先确认当前用户仍是 active 成员。
	messages, err := models.SearchConversationMessages(user.ID, conversationID, c.Query("q"), limit)
	if err != nil {
		writeSearchError(c, err)
		return
	}
	responses.Success(c, messages, "搜索消息成功")
}

// SearchMine 在当前用户参与的所有 active 会话中搜索文本消息。
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

	// 全局搜索通过 conversation_members 过滤会话范围，确保不会扫到用户无权访问的消息。
	messages, err := models.SearchMyMessages(user.ID, c.Query("q"), limit)
	if err != nil {
		writeSearchError(c, err)
		return
	}
	responses.Success(c, messages, "搜索消息成功")
}

func writeSearchError(c *gin.Context, err error) {
	// 搜索接口不把“没有结果”当错误；真正的错误只包括参数、权限和数据库异常。
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
