package models

import (
	"bytes"
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

	// staticRootDir 是上传资源在服务端本地磁盘的根目录。
	// main.go 会把 ./static 暴露为 /api/static，因此这里返回 URL 时会加上 /api 前缀。
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
// URL 使用 /api/static 开头，和 main.go 中 engine.Static("/api/static", "./static") 的暴露路径保持一致。
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
	// 扩展名先用于选择白名单，随后还会校验文件头；生产环境仍应增加病毒扫描和内容审核。
	dirName, err := resourceDirAndValidateExt(resourceType, ext)
	if err != nil {
		return ResourceDTO{}, err
	}
	// 扩展名由客户端提供，不能单独作为文件类型依据；读取文件头校验常见格式魔数。
	headerBytes := make([]byte, 512)
	n, readErr := io.ReadFull(src, headerBytes)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return ResourceDTO{}, ErrInvalidResourceFile
	}
	headerBytes = headerBytes[:n]
	if !matchesResourceSignature(ext, headerBytes) {
		return ResourceDTO{}, ErrInvalidResourceFile
	}

	// 每个用户单独一个目录，避免不同用户上传同名文件互相覆盖，也方便后续做用户维度清理。
	userDir := filepath.Join(staticRootDir, dirName, strconv.FormatInt(userID, 10))
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return ResourceDTO{}, fmt.Errorf("创建资源目录失败: %w", err)
	}

	// 使用纳秒时间戳作为文件名，并配合 O_EXCL 确保极端并发下不会覆盖已有文件。
	filename := strconv.FormatInt(time.Now().UnixNano(), 10) + ext
	dstPath := filepath.Join(userDir, filename)
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return ResourceDTO{}, fmt.Errorf("创建资源文件失败: %w", err)
	}
	saved := false
	defer func() {
		_ = dst.Close()
		if !saved {
			_ = os.Remove(dstPath)
		}
	}()

	// 文件头已经被读取，使用 MultiReader 把它拼回完整文件再写入磁盘。
	if _, err := io.Copy(dst, io.MultiReader(bytes.NewReader(headerBytes), src)); err != nil {
		return ResourceDTO{}, fmt.Errorf("保存资源文件失败: %w", err)
	}
	if err := dst.Close(); err != nil {
		return ResourceDTO{}, fmt.Errorf("关闭资源文件失败: %w", err)
	}
	saved = true

	return ResourceDTO{
		Type: resourceType,
		URL:  "/api/" + filepath.ToSlash(dstPath),
	}, nil
}

func matchesResourceSignature(ext string, data []byte) bool {
	switch ext {
	case ".jpg", ".jpeg":
		return len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff
	case ".png":
		return len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	case ".gif":
		return len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a")))
	case ".webp":
		return len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP"))
	case ".mp4", ".mov":
		return len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp"))
	case ".webm":
		return len(data) >= 4 && bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3})
	default:
		return false
	}
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
	// resourceType 决定子目录和后缀白名单。
	// 这里返回目录名而不是完整路径，让上层统一拼 userID 目录。
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
	// allowed 列表保持小而明确；新增格式时只需要改白名单，不需要碰上传主流程。
	for _, item := range allowed {
		if ext == item {
			return true
		}
	}
	return false
}
