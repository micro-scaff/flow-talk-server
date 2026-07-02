package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// DeviceController 处理当前用户设备记录。
type DeviceController struct{}

type UpsertDeviceRequest struct {
	DeviceID  string `json:"device_id" binding:"required"`
	Platform  string `json:"platform" binding:"required"`
	PushToken string `json:"push_token"`
}

// Upsert 新增或更新当前用户设备。
func (ctl DeviceController) Upsert(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	var req UpsertDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	device, err := models.UpsertUserDevice(user.ID, req.DeviceID, req.Platform, req.PushToken)
	if err != nil {
		writeDeviceError(c, err)
		return
	}
	responses.Success(c, device, "上报设备成功")
}

// Index 返回当前用户自己的设备列表。
func (ctl DeviceController) Index(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	devices, err := models.ListUserDevices(user.ID)
	if err != nil {
		writeDeviceError(c, err)
		return
	}
	responses.Success(c, devices, "获取设备列表成功")
}

// Delete 删除当前用户自己的某个设备，接口保持幂等。
func (ctl DeviceController) Delete(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	if err := models.DeleteUserDevice(user.ID, c.Param("device_id")); err != nil {
		writeDeviceError(c, err)
		return
	}
	responses.Success(c, nil, "删除设备成功")
}

func writeDeviceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, models.ErrInvalidDevice),
		errors.Is(err, models.ErrInvalidDevicePlatform):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
