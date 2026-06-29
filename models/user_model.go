package models

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const (
	// UserStatusDisabled 表示用户被禁用，旧 token 也不能继续访问接口。
	UserStatusDisabled = 0
	// UserStatusEnabled 表示用户正常可用。
	UserStatusEnabled = 1

	// AuthSourceLocal 表示用户由本服务注册登录。
	AuthSourceLocal = "local"
	// AuthSourceExternal 预留给后续接外部登录系统同步用户。
	AuthSourceExternal = "external"
)

var (
	// ErrInvalidCredentials 用于登录失败。
	// 用户不存在和密码错误统一返回这个错误，避免接口暴露“账号是否存在”。
	ErrInvalidCredentials = errors.New("账号或密码错误")
	// ErrUserDisabled 表示用户存在但已禁用。
	ErrUserDisabled = errors.New("用户已禁用")
	// ErrUserNotFound 表示数据库没有查到用户。
	ErrUserNotFound = errors.New("用户不存在")
	// ErrUsernameTaken 表示 username 唯一键冲突。
	ErrUsernameTaken = errors.New("用户名已存在")
	// ErrValidation 表示调用方传入的必要参数为空或不合法。
	ErrValidation = errors.New("参数校验失败")
)

// User 映射 users 表，是 MVC 中的用户 Model。
// GORM tags 用来明确字段名和主键，避免数据库字段和 Go 字段命名不一致时出现隐式猜测。
type User struct {
	ID int64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	// ExternalID 和 AvatarURL 是数据库可空字段，用指针可以让 GORM 写入 NULL，而不是空字符串。
	ExternalID *string `gorm:"column:external_id" json:"external_id,omitempty"`
	// Username 是本地登录账号，对应 users.username 唯一索引。
	Username string `gorm:"column:username" json:"username"`
	// Password 当前阶段仍是明文密码，仅用于打通登录流程；json:"-" 保证不会输出到接口响应。
	Password string `gorm:"column:password" json:"-"`
	// Nickname 是 IM 展示昵称；注册未传时默认使用 username。
	Nickname string `gorm:"column:nickname" json:"nickname"`
	// AvatarURL 是头像地址，空值写入 NULL。
	AvatarURL *string `gorm:"column:avatar_url" json:"avatar_url,omitempty"`
	// AuthSource 标记用户来源：local 表示本地注册，external 预留给外部系统。
	AuthSource string `gorm:"column:auth_source" json:"auth_source"`
	// Status 标记用户状态，0 禁用，1 正常。
	Status int `gorm:"column:status" json:"status"`
}

// TableName 明确指定表名，避免 GORM 自动复数规则和现有表名产生偏差。
func (User) TableName() string {
	return "users"
}

// UserDTO 是接口返回给前端的用户结构。
// 不暴露 password，避免任何响应意外泄露密码字段。
type UserDTO struct {
	ID         int64  `json:"id"`
	ExternalID string `json:"external_id,omitempty"`
	Username   string `json:"username"`
	Nickname   string `json:"nickname"`
	AvatarURL  string `json:"avatar_url,omitempty"`
	AuthSource string `json:"auth_source"`
	Status     int    `json:"status"`
}

// ToDTO 把数据库模型转换成接口输出模型。
func (u User) ToDTO() UserDTO {
	// 数据库模型里有密码等内部字段，对外统一通过 DTO 过滤。
	// 指针字段通过 stringValue 转成空字符串，方便前端直接消费。
	return UserDTO{
		ID:         u.ID,
		ExternalID: stringValue(u.ExternalID),
		Username:   u.Username,
		Nickname:   u.Nickname,
		AvatarURL:  stringValue(u.AvatarURL),
		AuthSource: u.AuthSource,
		Status:     u.Status,
	}
}

// CreateLocalUser 创建本地注册用户。
// 当前阶段为了先跑通接口仍保存明文密码，后续应该替换为 password_hash。
func CreateLocalUser(username string, password string, nickname string, avatarURL string) (User, error) {
	// 用户名、昵称、头像地址都去掉首尾空格，避免 "alice" 和 " alice " 被当成不同账号。
	username = strings.TrimSpace(username)
	nickname = strings.TrimSpace(nickname)
	avatarURL = strings.TrimSpace(avatarURL)
	// 注册最少需要 username/password；nickname 可以不传。
	if username == "" || password == "" {
		return User{}, ErrValidation
	}
	// 没有昵称时使用 username，保证 users.nickname 的 NOT NULL 约束始终满足。
	if nickname == "" {
		nickname = username
	}

	// 本地用户 external_id 不赋值，GORM 会写入 NULL。
	// 这点很重要：users.external_id 有唯一索引，多条 NULL 在 MySQL 中允许共存，空字符串则会冲突。
	user := User{
		Username:   username,
		Password:   password,
		Nickname:   nickname,
		AvatarURL:  optionalString(avatarURL),
		AuthSource: AuthSourceLocal,
		Status:     UserStatusEnabled,
	}
	// Create 会执行 INSERT，并把自增 ID 回填到 user.ID。
	if err := DB.Create(&user).Error; err != nil {
		if isDuplicateKey(err) {
			return User{}, ErrUsernameTaken
		}
		return User{}, fmt.Errorf("创建用户失败: %w", err)
	}
	return user, nil
}

// LoginUser 校验用户名和密码，成功后返回完整用户信息。
func LoginUser(username string, password string) (User, error) {
	username = strings.TrimSpace(username)
	// 登录参数为空时直接按凭证错误处理，不暴露更细的字段错误。
	if username == "" || password == "" {
		return User{}, ErrInvalidCredentials
	}

	user, err := FindUserByUsername(username)
	if err != nil {
		// 用户不存在时也返回 ErrInvalidCredentials，避免账号枚举。
		if errors.Is(err, ErrUserNotFound) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}
	if user.Status != UserStatusEnabled {
		return User{}, ErrUserDisabled
	}
	// 当前阶段是明文比较；后续改 password_hash 后，这里应替换为哈希校验。
	if user.Password != password {
		return User{}, ErrInvalidCredentials
	}
	return user, nil
}

// FindUserByUsername 根据 username 查询用户，供登录使用。
func FindUserByUsername(username string) (User, error) {
	var user User
	// GORM 的 First 会自动追加 LIMIT 1。
	// Where 使用占位符，避免 SQL 注入。
	err := DB.Where("username = ?", strings.TrimSpace(username)).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("查询用户失败: %w", err)
	}
	return user, nil
}

// FindUserByID 根据内部用户 ID 查询用户，供 JWT 鉴权后刷新用户状态使用。
func FindUserByID(id int64) (User, error) {
	var user User
	// 使用主键查询，等价于 WHERE id = ? LIMIT 1。
	err := DB.First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("查询用户失败: %w", err)
	}
	return user, nil
}

// ListUsers 返回用户列表，给 controllers/user_controller.go 的演示接口使用。
func ListUsers() ([]UserDTO, error) {
	var users []User
	// 后注册的用户排在前面，方便后台调试时看到最新数据。
	if err := DB.Order("id desc").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询用户列表失败: %w", err)
	}

	// 统一转换成 DTO，确保列表接口也不会泄露 password 字段。
	result := make([]UserDTO, 0, len(users))
	for _, user := range users {
		result = append(result, user.ToDTO())
	}
	return result, nil
}

// isDuplicateKey 判断 MySQL 唯一键冲突，主要用于 username 重复注册。
func isDuplicateKey(err error) bool {
	// MySQL 唯一键冲突通常包含 Duplicate entry 或 Error 1062。
	// 这里不依赖具体驱动错误类型，保持 model 层判断简单直接。
	message := err.Error()
	return strings.Contains(message, "Duplicate entry") || strings.Contains(message, "Error 1062")
}

func optionalString(value string) *string {
	// 空字符串在数据库中存为 NULL，适合 external_id、avatar_url 这种可选字段。
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	// DTO 对外返回 string，比 *string 更方便前端展示；NULL 统一变成空字符串。
	if value == nil {
		return ""
	}
	return *value
}
