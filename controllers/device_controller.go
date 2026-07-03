package controllers

import (
	"encoding/json"
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

// DeviceController 处理当前用户设备记录。
// 设备接口只允许当前登录用户上报自己的设备数据，避免越权管理其它人的设备。
type DeviceController struct{}

// UpsertDeviceRequest 是设备上报请求。
type UpsertDeviceRequest struct {
	UserID int64           `json:"user_id" binding:"required"`
	Data   json.RawMessage `json:"data" binding:"required"`
}

// Upsert 新增或更新当前用户设备上报数据。
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
	if req.UserID != user.ID {
		responses.Error(c, http.StatusForbidden, "无权上报其它用户设备")
		return
	}

	// 重复上报同一个用户时不会新增记录，而是更新完整 JSON 数据和 updated_at。
	device, err := models.UpsertUserDevice(req.UserID, req.Data)
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

// Delete 删除当前用户自己的设备上报数据，接口保持幂等。
func (ctl DeviceController) Delete(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	if err := models.DeleteUserDevice(user.ID); err != nil {
		writeDeviceError(c, err)
		return
	}
	responses.Success(c, nil, "删除设备成功")
}

func writeDeviceError(c *gin.Context, err error) {
	// 设备相关错误目前只有参数类和内部错误两类。
	// 这里不暴露数据库错误细节，避免把索引名、SQL 等内部信息返回给客户端。
	switch {
	case errors.Is(err, models.ErrInvalidDevice):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
