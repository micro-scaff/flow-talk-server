package models

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// ResourceTypeImage 表示用户上传的是聊天图片资源。
	ResourceTypeImage = "image"
	// ResourceTypeVideo 表示用户上传的是聊天视频资源。
	ResourceTypeVideo = "video"

	staticRootDir = "static"
)

var (
	// ErrInvalidResourceType 表示上传资源类型不是 image/video。
	ErrInvalidResourceType = errors.New("无效资源类型")
	// ErrInvalidResourceFile 表示文件名、后缀或文件内容不符合当前版本规则。
	ErrInvalidResourceFile = errors.New("无效资源文件")
	// ErrInvalidAvatarBase64 表示注册头像不是合法 base64 字符串。
	ErrInvalidAvatarBase64 = errors.New("无效头像 base64")
)

// ResourceDTO 是资源上传成功后的返回结构。
// URL 使用 /static 开头，和 main.go 中 engine.Static("/static", "./static") 的暴露路径保持一致。
type ResourceDTO struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// SaveUploadedResource 按文档规则保存图片或视频：
// 图片：static/images/用户id/上传时间戳.图片后缀
// 视频：static/videos/用户id/上传时间戳.视频后缀
func SaveUploadedResource(userID int64, resourceType string, header *multipart.FileHeader, src multipart.File) (ResourceDTO, error) {
	resourceType = strings.TrimSpace(resourceType)
	if userID <= 0 {
		return ResourceDTO{}, ErrInvalidMember
	}
	if header == nil || src == nil {
		return ResourceDTO{}, ErrInvalidResourceFile
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	dirName, err := resourceDirAndValidateExt(resourceType, ext)
	if err != nil {
		return ResourceDTO{}, err
	}

	userDir := filepath.Join(staticRootDir, dirName, strconv.FormatInt(userID, 10))
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return ResourceDTO{}, fmt.Errorf("创建资源目录失败: %w", err)
	}

	filename := strconv.FormatInt(time.Now().UnixNano(), 10) + ext
	dstPath := filepath.Join(userDir, filename)
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return ResourceDTO{}, fmt.Errorf("创建资源文件失败: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return ResourceDTO{}, fmt.Errorf("保存资源文件失败: %w", err)
	}

	return ResourceDTO{
		Type: resourceType,
		URL:  "/" + filepath.ToSlash(dstPath),
	}, nil
}

// NormalizeAvatarBase64 校验注册头像 base64。
// 文档要求头像“直接转换为 base64 存储”，所以这里不落文件，只确认字符串可解码后原样保存。
func NormalizeAvatarBase64(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}

	encoded := value
	if comma := strings.Index(value, ","); comma >= 0 {
		// 支持前端常见 data URL：data:image/png;base64,xxxx。
		// 实际解码时只取逗号后的 base64 数据，但数据库仍保存原始字符串，方便前端直接展示。
		encoded = value[comma+1:]
	}
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return "", ErrInvalidAvatarBase64
	}
	if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
		if _, rawErr := base64.RawStdEncoding.DecodeString(encoded); rawErr != nil {
			return "", ErrInvalidAvatarBase64
		}
	}
	return value, nil
}

func resourceDirAndValidateExt(resourceType string, ext string) (string, error) {
	if ext == "" {
		return "", ErrInvalidResourceFile
	}

	switch resourceType {
	case ResourceTypeImage:
		if !allowedExt(ext, []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}) {
			return "", ErrInvalidResourceFile
		}
		return "images", nil
	case ResourceTypeVideo:
		if !allowedExt(ext, []string{".mp4", ".mov", ".webm"}) {
			return "", ErrInvalidResourceFile
		}
		return "videos", nil
	default:
		return "", ErrInvalidResourceType
	}
}

func allowedExt(ext string, allowed []string) bool {
	for _, item := range allowed {
		if ext == item {
			return true
		}
	}
	return false
}
