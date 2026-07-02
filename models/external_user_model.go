package models

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// FindUserByExternalID 根据外部身份 ID 查询内部用户。
func FindUserByExternalID(externalID string) (User, error) {
	// external_id 是外部身份和内部用户的唯一映射键。
	// 空 external_id 不能参与查询，否则容易把 NULL/空字符串语义混在一起。
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return User{}, ErrValidation
	}

	var user User
	err := DB.Where("external_id = ?", externalID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("查询外部用户失败: %w", err)
	}
	return user, nil
}

// SyncExternalUser 把外部身份资料落到内部 users 表。
// external_id 是稳定映射键；昵称和头像每次登录都可以同步刷新。
func SyncExternalUser(profile ExternalUserProfile) (User, error) {
	// verifier 返回的资料进入数据库前统一清理空格。
	// 这样相同外部用户不会因为昵称或账号前后空格产生不可预期展示。
	profile.ExternalID = strings.TrimSpace(profile.ExternalID)
	profile.Username = strings.TrimSpace(profile.Username)
	profile.Nickname = strings.TrimSpace(profile.Nickname)
	profile.AvatarURL = strings.TrimSpace(profile.AvatarURL)
	if profile.ExternalID == "" {
		return User{}, ErrValidation
	}
	if profile.Username == "" {
		profile.Username = profile.ExternalID
	}
	if profile.Nickname == "" {
		profile.Nickname = profile.Username
	}

	// 已存在的外部用户只刷新展示资料和状态，不改变内部 ID。
	// 内部 user_id 是会话、消息、设备等所有 IM 数据的稳定关联键。
	existing, err := FindUserByExternalID(profile.ExternalID)
	if err == nil {
		updates := map[string]any{
			"nickname":   profile.Nickname,
			"avatar_url": optionalString(profile.AvatarURL),
			"status":     UserStatusEnabled,
		}
		if err := DB.Model(&existing).Updates(updates).Error; err != nil {
			return User{}, fmt.Errorf("同步外部用户失败: %w", err)
		}
		existing.Nickname = profile.Nickname
		existing.AvatarURL = optionalString(profile.AvatarURL)
		existing.Status = UserStatusEnabled
		return existing, nil
	}
	if !errors.Is(err, ErrUserNotFound) {
		return User{}, err
	}

	// 新外部用户也落到 users 表，后续所有 IM 能力都以内部 users.id 为准。
	// Password 为空，因为外部用户不通过本地账号密码登录。
	user := User{
		ExternalID: optionalString(profile.ExternalID),
		Username:   profile.Username,
		Password:   "",
		Nickname:   profile.Nickname,
		AvatarURL:  optionalString(profile.AvatarURL),
		AuthSource: AuthSourceExternal,
		Status:     UserStatusEnabled,
	}
	if err := DB.Create(&user).Error; err != nil {
		if isDuplicateKey(err) {
			return User{}, ErrUsernameTaken
		}
		return User{}, fmt.Errorf("创建外部用户失败: %w", err)
	}
	return user, nil
}

// LoginExternalUser 校验外部 token 并同步内部用户。
func LoginExternalUser(provider string, accessToken string) (User, error) {
	// provider 只决定 token 如何校验；校验通过后必须统一同步成内部用户。
	verifier, err := verifierForProvider(provider)
	if err != nil {
		return User{}, err
	}
	profile, err := verifier.Verify(accessToken)
	if err != nil {
		return User{}, err
	}
	user, err := SyncExternalUser(profile)
	if err != nil {
		return User{}, err
	}
	// 即使外部 token 有效，只要内部用户被禁用，也不能继续换取 IM token。
	if user.Status != UserStatusEnabled {
		return User{}, ErrUserDisabled
	}
	return user, nil
}
