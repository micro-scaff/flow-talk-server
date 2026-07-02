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
	switch platform {
	case DevicePlatformWeb, DevicePlatformIOS, DevicePlatformAndroid, DevicePlatformDesktop:
		return nil
	default:
		return ErrInvalidDevicePlatform
	}
}

func findUserDevice(userID int64, deviceID string) (UserDevice, error) {
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
