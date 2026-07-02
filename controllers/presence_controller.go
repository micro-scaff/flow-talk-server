package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// PresenceController 处理在线状态查询。
type PresenceController struct {
	Hub *models.WSHub
}

type BatchPresenceRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
}

func (ctl PresenceController) Show(c *gin.Context) {
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
	switch {
	case errors.Is(err, models.ErrInvalidMember),
		errors.Is(err, models.ErrInvalidDevice):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
