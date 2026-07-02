package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// GroupController 处理群资料和群成员管理接口。
// 所有群管理权限都下沉到 models/conversation_model.go，controller 只做参数解析和响应转换。
type GroupController struct{}

// AddMembersRequest 是添加群成员请求。
// user_ids 可以包含重复 ID，model 层会统一去重和校验用户是否存在。
type AddMembersRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
}

// UpdateMemberRoleRequest 是设置管理员/普通成员的请求。
// owner 角色不能通过这个接口设置，避免绕开群主转让流程。
type UpdateMemberRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// UpdateGroupProfileRequest 是修改群资料请求。
// title 必填，avatar_url 可为空；空头像会在数据库中保存为 NULL。
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

	// 添加成员支持重新激活 left/removed 成员。
	// 具体权限矩阵：owner/admin 可添加，member 不可添加。
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

	// 移除成员时禁止移除群主；管理员只能移除普通成员。
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

	// 群主不能直接退出，避免群聊没有 owner。
	// 后续如果实现群主转让，可以先转让再退出。
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

	// 只有 owner 可以设置 admin/member，且不能修改 owner 本身。
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

	// 群资料修改允许 owner/admin 操作，普通成员只读。
	conversation, err := models.UpdateGroupProfile(user.ID, conversationID, req.Title, req.AvatarURL)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	responses.Success(c, conversation, "修改群资料成功")
}

func writeGroupError(c *gin.Context, err error) {
	// 这里把群管理领域错误转成对前端稳定的 HTTP 语义。
	// 权限错误统一返回 403；“只能群聊使用”属于调用场景错误，返回 400。
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
