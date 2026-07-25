package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// PresenceController 处理在线状态查询。
// 在线状态来源由 PresenceProvider 决定：默认本机 Hub，启用 Redis 后为全局 presence。
type PresenceController struct {
	PresenceProvider models.PresenceProvider
}

// PresenceRequest 是单个用户在线状态查询请求。
type PresenceRequest struct {
	UserID int64 `json:"user_id" binding:"required"`
}

// BatchPresenceRequest 是在线人员查询请求。
// Type 支持 all、online、offline、specified；specified 需要同时传 user_ids。
type BatchPresenceRequest struct {
	Type    models.PresenceQueryType `json:"type"`
	UserIDs []int64                  `json:"user_ids"`
}

// Show 查询单个用户在线状态。
func (ctl PresenceController) Show(c *gin.Context) {
	// 当前版本只要求登录后可查在线状态，暂不限制“只能查好友/同会话成员”。
	// 后续有好友关系或组织权限时，可以在这里补更细的访问控制。
	if _, ok := currentUserOrUnauthorized(c); !ok {
		return
	}

	var req PresenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	presence, err := models.GetUserPresence(ctl.PresenceProvider, req.UserID)
	if err != nil {
		writePresenceError(c, err)
		return
	}
	responses.Success(c, presence, "获取在线状态成功")
}

// Batch 批量查询用户资料和在线状态，前端可直接用返回结果渲染人员列表。
func (ctl PresenceController) Batch(c *gin.Context) {
	if _, ok := currentUserOrUnauthorized(c); !ok {
		return
	}

	var req BatchPresenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	queryType := req.Type
	// 兼容旧客户端：历史请求体只有 user_ids，等价于 specified。
	if queryType == "" && len(req.UserIDs) > 0 {
		queryType = models.PresenceQuerySpecified
	}

	presences, err := models.QueryUserPresence(ctl.PresenceProvider, queryType, req.UserIDs)
	if err != nil {
		writePresenceError(c, err)
		return
	}
	responses.Success(c, presences, "获取在线人员成功")
}

func writePresenceError(c *gin.Context, err error) {
	// Presence 查询横跨 Hub 和设备表。
	// 对客户端而言，无效用户 ID 和无效设备状态都属于参数问题，其它数据库异常统一走 500。
	switch {
	case errors.Is(err, models.ErrInvalidMember),
		errors.Is(err, models.ErrInvalidDevice),
		errors.Is(err, models.ErrInvalidPresenceQuery):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
