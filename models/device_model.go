package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DevicePlatformWeb     = "web"
	DevicePlatformIOS     = "ios"
	DevicePlatformAndroid = "android"
	DevicePlatformDesktop = "desktop"
)

var (
	ErrInvalidDevice         = errors.New("无效设备")
	ErrInvalidDevicePlatform = errors.New("无效设备平台")
)

// UserDevice 映射 user_devices 表。
// 设备记录用于离线同步和后续推送 token 保存，不等同于当前在线连接。
type UserDevice struct {
	ID         int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID     int64      `gorm:"column:user_id" json:"user_id"`
	DeviceID   string     `gorm:"column:device_id" json:"device_id"`
	Platform   string     `gorm:"column:platform" json:"platform"`
	PushToken  *string    `gorm:"column:push_token" json:"push_token,omitempty"`
	LastSeenAt *time.Time `gorm:"column:last_seen_at" json:"last_seen_at,omitempty"`
	CreatedAt  time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (UserDevice) TableName() string {
	return "user_devices"
}

type UserDeviceDTO struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	DeviceID   string `json:"device_id"`
	Platform   string `json:"platform"`
	PushToken  string `json:"push_token,omitempty"`
	LastSeenAt string `json:"last_seen_at,omitempty"`
}

func (d UserDevice) ToDTO() UserDeviceDTO {
	// DTO 使用字符串时间和字符串 push_token，避免前端处理 null 指针。
	return UserDeviceDTO{
		ID:         d.ID,
		UserID:     d.UserID,
		DeviceID:   d.DeviceID,
		Platform:   d.Platform,
		PushToken:  stringValue(d.PushToken),
		LastSeenAt: timeString(d.LastSeenAt),
	}
}

// UpsertUserDevice 新增或更新当前用户的设备记录。
// 唯一性由数据库的 (user_id, device_id) 约束保证，重复上报只更新平台、推送 token 和活跃时间。
func UpsertUserDevice(userID int64, deviceID string, platform string, pushToken string) (UserDeviceDTO, error) {
	// 入参清理放在 model 层，保证 HTTP、WebSocket 或后台任务复用这个方法时规则一致。
	deviceID = strings.TrimSpace(deviceID)
	platform = strings.TrimSpace(platform)
	pushToken = strings.TrimSpace(pushToken)
	if userID <= 0 || deviceID == "" {
		return UserDeviceDTO{}, ErrInvalidDevice
	}
	if err := validateDevicePlatform(platform); err != nil {
		return UserDeviceDTO{}, err
	}

	now := time.Now()
	// Create + OnConflict 让新增和更新走同一条数据库语句。
	// 这样客户端重复上报、网络重试都不会产生重复设备记录。
	device := UserDevice{
		UserID:     userID,
		DeviceID:   deviceID,
		Platform:   platform,
		PushToken:  optionalString(pushToken),
		LastSeenAt: &now,
	}

	err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "device_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"platform":     platform,
			"push_token":   optionalString(pushToken),
			"last_seen_at": now,
		}),
	}).Create(&device).Error
	if err != nil {
		return UserDeviceDTO{}, fmt.Errorf("上报设备失败: %w", err)
	}

	// MySQL ON DUPLICATE KEY UPDATE 不一定会把已存在记录的 id 回填到当前结构体。
	// 因此 upsert 后再查一次，保证响应里包含真实数据库记录。
	saved, err := findUserDevice(userID, deviceID)
	if err != nil {
		return UserDeviceDTO{}, err
	}
	return saved.ToDTO(), nil
}

func ListUserDevices(userID int64) ([]UserDeviceDTO, error) {
	if userID <= 0 {
		return nil, ErrInvalidDevice
	}

	// 最近活跃设备排在前面，客户端更容易展示当前使用设备或最近登录设备。
	var devices []UserDevice
	err := DB.Where("user_id = ?", userID).
		Order("COALESCE(last_seen_at, updated_at) DESC").
		Find(&devices).Error
	if err != nil {
		return nil, fmt.Errorf("查询设备列表失败: %w", err)
	}

	result := make([]UserDeviceDTO, 0, len(devices))
	for _, device := range devices {
		result = append(result, device.ToDTO())
	}
	return result, nil
}

// DeleteUserDevice 删除当前用户自己的设备。
// 删除不存在的设备返回成功，方便客户端重复调用。
func DeleteUserDevice(userID int64, deviceID string) error {
	deviceID = strings.TrimSpace(deviceID)
	if userID <= 0 || deviceID == "" {
		return ErrInvalidDevice
	}
	// Delete 带 user_id 条件，确保路径里的 device_id 不能删除其它用户设备。
	// GORM 删除 0 行不会返回错误，刚好满足接口幂等要求。
	err := DB.Where("user_id = ? AND device_id = ?", userID, deviceID).Delete(&UserDevice{}).Error
	if err != nil {
		return fmt.Errorf("删除设备失败: %w", err)
	}
	return nil
}

// TouchUserDevice 更新设备最近活跃时间。
// 如果设备还没通过 HTTP 上报，则不自动创建，避免平台信息缺失的脏数据。
func TouchUserDevice(userID int64, deviceID string) error {
	deviceID = strings.TrimSpace(deviceID)
	if userID <= 0 || deviceID == "" {
		return ErrInvalidDevice
	}
	now := time.Now()
	// 这里只更新已存在设备，不自动插入。
	// 原因是自动插入会缺少 platform，容易留下无法判断来源的设备记录。
	err := DB.Model(&UserDevice{}).
		Where("user_id = ? AND device_id = ?", userID, deviceID).
		Update("last_seen_at", now).Error
	if err != nil {
		return fmt.Errorf("更新设备活跃时间失败: %w", err)
	}
	return nil
}

func LatestDeviceSeenAt(userID int64) (*time.Time, error) {
	if userID <= 0 {
		return nil, ErrInvalidDevice
	}
	// 在线状态接口会使用这个值作为离线用户的最近活跃时间。
	// 没有设备记录时返回 nil，不把“没有上报过设备”当成错误。
	var device UserDevice
	err := DB.Where("user_id = ? AND last_seen_at IS NOT NULL", userID).
		Order("last_seen_at DESC").
		First(&device).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询设备活跃时间失败: %w", err)
	}
	return device.LastSeenAt, nil
}

func validateDevicePlatform(platform string) error {
	// platform 使用白名单，不直接信任客户端字符串。
	// 这能保证数据库 enum、接口文档和业务代码三处保持一致。
	switch platform {
	case DevicePlatformWeb, DevicePlatformIOS, DevicePlatformAndroid, DevicePlatformDesktop:
		return nil
	default:
		return ErrInvalidDevicePlatform
	}
}

func findUserDevice(userID int64, deviceID string) (UserDevice, error) {
	// 设备查询永远带 user_id，device_id 只在用户维度内唯一。
	var device UserDevice
	err := DB.Where("user_id = ? AND device_id = ?", userID, deviceID).First(&device).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return UserDevice{}, ErrInvalidDevice
	}
	if err != nil {
		return UserDevice{}, fmt.Errorf("查询设备失败: %w", err)
	}
	return device, nil
}
