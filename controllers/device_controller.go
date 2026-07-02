package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// DeviceController 处理当前用户设备记录。
// 设备接口只操作“当前登录用户自己的设备”，不会允许客户端传 user_id，避免越权管理其它人的设备。
type DeviceController struct{}

// UpsertDeviceRequest 是设备上报请求。
// device_id 由客户端生成或设备侧提供；服务端使用 user_id + device_id 作为唯一设备。
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

	// controller 只负责 JSON binding；device_id 去空格、platform 白名单、upsert 规则都放在 model 层。
	var req UpsertDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
		return
	}

	// 重复上报同一个设备时不会新增记录，而是更新平台、push_token 和 last_seen_at。
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

	// 设备列表用于客户端离线同步和调试当前账号的多端信息，只返回当前 token 对应用户的数据。
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

	// device_id 放在 path 中，model 层仍会带 user_id 条件删除，确保不能删其它用户设备。
	if err := models.DeleteUserDevice(user.ID, c.Param("device_id")); err != nil {
		writeDeviceError(c, err)
		return
	}
	responses.Success(c, nil, "删除设备成功")
}

func writeDeviceError(c *gin.Context, err error) {
	// 设备相关错误目前只有参数类和内部错误两类。
	// 这里不暴露数据库错误细节，避免把索引名、SQL 等内部信息返回给客户端。
	switch {
	case errors.Is(err, models.ErrInvalidDevice),
		errors.Is(err, models.ErrInvalidDevicePlatform):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
