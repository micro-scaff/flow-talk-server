package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"flow-talk/middlewares"
	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// ConversationController 处理会话相关接口。
type ConversationController struct{}

// CreateDirectConversationRequest 是创建或获取单聊的请求体。
type CreateDirectConversationRequest struct {
	TargetUserID int64 `json:"target_user_id" binding:"required"`
}

// CreateGroupConversationRequest 是创建群聊的请求体。
// MemberIDs 不需要包含当前用户；model 层会自动把创建者补进成员列表。
type CreateGroupConversationRequest struct {
	Title     string  `json:"title" binding:"required"`
	AvatarURL string  `json:"avatar_url"`
	MemberIDs []int64 `json:"member_ids"`
}

// Index 返回当前用户参与的会话列表。
func (ctl ConversationController) Index(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	conversations, err := models.ListConversations(user.ID)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	responses.Success(c, conversations, "获取会话列表成功")
}

// Show 返回单个会话详情。
func (ctl ConversationController) Show(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	// 路径参数统一在 controller 解析，model 层只接收明确的 int64 ID。
	conversationID, err := parseIDParam(c, "conversation_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	conversation, err := models.GetConversationDetail(user.ID, conversationID)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	responses.Success(c, conversation, "获取会话详情成功")
}

// CreateDirect 创建或获取当前用户和目标用户之间的单聊会话。
func (ctl ConversationController) CreateDirect(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	var req CreateDirectConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	conversation, err := models.GetOrCreateDirectConversation(user.ID, req.TargetUserID)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	responses.Success(c, conversation, "获取单聊会话成功")
}

// CreateGroup 创建群聊会话。
func (ctl ConversationController) CreateGroup(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	var req CreateGroupConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	conversation, err := models.CreateGroupConversation(user.ID, req.Title, req.AvatarURL, req.MemberIDs)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	responses.Success(c, conversation, "创建群聊成功")
}

func currentUserOrUnauthorized(c *gin.Context) (models.User, bool) {
	user, ok := middlewares.CurrentUser(c)
	if !ok {
		responses.Error(c, http.StatusUnauthorized, "未登录或登录已失效")
		return models.User{}, false
	}
	return user, true
}

func parseIDParam(c *gin.Context, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

// writeConversationError 把 model 层领域错误翻译为 HTTP 状态码。
// 这样 controller 方法里不用重复写 switch，也不会把内部错误直接暴露给客户端。
func writeConversationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, models.ErrValidation),
		errors.Is(err, models.ErrInvalidMember),
		errors.Is(err, models.ErrInvalidConversationType),
		errors.Is(err, models.ErrCannotCreateDirectWithSelf):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	case errors.Is(err, models.ErrConversationForbidden):
		responses.Error(c, http.StatusForbidden, "无权访问该会话")
	case errors.Is(err, models.ErrConversationNotFound):
		responses.Error(c, http.StatusNotFound, "会话不存在")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
