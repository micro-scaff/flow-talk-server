package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// GroupController 处理群资料和群成员管理接口。
type GroupController struct{}

type AddMembersRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
}

type UpdateMemberRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

type UpdateGroupProfileRequest struct {
	Title     string `json:"title" binding:"required"`
	AvatarURL string `json:"avatar_url"`
}

func (ctl GroupController) AddMembers(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}
	conversationID, err := parseIDParam(c, "conversation_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	var req AddMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	members, err := models.AddGroupMembers(user.ID, conversationID, req.UserIDs)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	responses.Success(c, members, "添加群成员成功")
}

func (ctl GroupController) RemoveMember(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}
	conversationID, err := parseIDParam(c, "conversation_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}
	targetUserID, err := parseIDParam(c, "user_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	if err := models.RemoveGroupMember(user.ID, conversationID, targetUserID); err != nil {
		writeGroupError(c, err)
		return
	}
	responses.Success(c, nil, "移除群成员成功")
}

func (ctl GroupController) Leave(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}
	conversationID, err := parseIDParam(c, "conversation_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	if err := models.LeaveGroup(user.ID, conversationID); err != nil {
		writeGroupError(c, err)
		return
	}
	responses.Success(c, nil, "退出群聊成功")
}

func (ctl GroupController) UpdateMemberRole(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}
	conversationID, err := parseIDParam(c, "conversation_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}
	targetUserID, err := parseIDParam(c, "user_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	var req UpdateMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	member, err := models.UpdateMemberRole(user.ID, conversationID, targetUserID, req.Role)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	responses.Success(c, member, "更新成员角色成功")
}

func (ctl GroupController) UpdateProfile(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}
	conversationID, err := parseIDParam(c, "conversation_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	var req UpdateGroupProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	conversation, err := models.UpdateGroupProfile(user.ID, conversationID, req.Title, req.AvatarURL)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	responses.Success(c, conversation, "修改群资料成功")
}

func writeGroupError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, models.ErrValidation),
		errors.Is(err, models.ErrInvalidMember),
		errors.Is(err, models.ErrInvalidMemberRole),
		errors.Is(err, models.ErrCannotRemoveOwner),
		errors.Is(err, models.ErrOwnerCannotLeave):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	case errors.Is(err, models.ErrPermissionDenied),
		errors.Is(err, models.ErrConversationForbidden):
		responses.Error(c, http.StatusForbidden, "无权操作该群聊")
	case errors.Is(err, models.ErrConversationNotFound):
		responses.Error(c, http.StatusNotFound, "会话不存在")
	case errors.Is(err, models.ErrGroupOnly):
		responses.Error(c, http.StatusBadRequest, "该操作只支持群聊")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
