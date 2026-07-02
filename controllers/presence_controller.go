package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// PresenceController 处理在线状态查询。
// 当前在线状态来自本进程 WSHub；last_seen_at 来自 user_devices，能兼容用户离线后的最近活跃时间。
type PresenceController struct {
	Hub *models.WSHub
}

// BatchPresenceRequest 是批量在线状态查询请求。
// model 层会去掉非法 ID 和重复 ID，保证返回结果稳定。
type BatchPresenceRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
}

// Show 查询单个用户在线状态。
func (ctl PresenceController) Show(c *gin.Context) {
	// 当前版本只要求登录后可查在线状态，暂不限制“只能查好友/同会话成员”。
	// 后续有好友关系或组织权限时，可以在这里补更细的访问控制。
	if _, ok := currentUserOrUnauthorized(c); !ok {
		return
	}

	userID, err := parseIDParam(c, "user_id")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	presence, err := models.GetUserPresence(ctl.Hub, userID)
	if err != nil {
		writePresenceError(c, err)
		return
	}
	responses.Success(c, presence, "获取在线状态成功")
}

// Batch 批量查询用户在线状态，适合会话列表或成员列表一次性渲染在线标记。
func (ctl PresenceController) Batch(c *gin.Context) {
	if _, ok := currentUserOrUnauthorized(c); !ok {
		return
	}

	var req BatchPresenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	presences, err := models.BatchUserPresence(ctl.Hub, req.UserIDs)
	if err != nil {
		writePresenceError(c, err)
		return
	}
	responses.Success(c, presences, "获取在线状态成功")
}

func writePresenceError(c *gin.Context, err error) {
	// Presence 查询横跨 Hub 和设备表。
	// 对客户端而言，无效用户 ID 和无效设备状态都属于参数问题，其它数据库异常统一走 500。
	switch {
	case errors.Is(err, models.ErrInvalidMember),
		errors.Is(err, models.ErrInvalidDevice):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
