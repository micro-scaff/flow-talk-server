package controllers

import (
	"errors"
	"net/http"

	"flow-talk/models"
	"flow-talk/responses"

	"github.com/gin-gonic/gin"
)

const (
	// maxResourceUploadBytes 是单个资源上传请求体上限。
	// 当前先给图片和短视频一个保守上限，避免异常大文件直接打满磁盘或内存。
	maxResourceUploadBytes = 100 << 20 // 100 MiB
)

// ResourceController 处理图片、视频等静态资源上传。
type ResourceController struct{}

// Upload 接收 multipart/form-data 上传，并按 docs/图片、视频等资源存放.md 的规则写入 static 目录。
func (ctl ResourceController) Upload(c *gin.Context) {
	user, ok := currentUserOrUnauthorized(c)
	if !ok {
		return
	}

	// 先限制请求体大小，再解析 multipart，避免大文件在解析阶段占用过多资源。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxResourceUploadBytes)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		responses.Error(c, http.StatusBadRequest, "请选择上传文件")
		return
	}
	defer file.Close()

	// type 使用表单字段传入，只允许 image/video。
	// 具体后缀白名单和目录规则放在 model 层统一维护。
	resource, err := models.SaveUploadedResource(user.ID, c.PostForm("type"), header, file)
	if err != nil {
		writeResourceError(c, err)
		return
	}
	responses.Success(c, resource, "上传资源成功")
}

func writeResourceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, models.ErrInvalidResourceType),
		errors.Is(err, models.ErrInvalidResourceFile),
		errors.Is(err, models.ErrInvalidMember):
		responses.Error(c, http.StatusBadRequest, "参数校验失败")
	default:
		responses.Error(c, http.StatusInternalServerError, "服务器内部错误")
	}
}
