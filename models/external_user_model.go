package models

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// FindUserByExternalID 根据外部身份 ID 查询内部用户。
func FindUserByExternalID(externalID string) (User, error) {
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
	if user.Status != UserStatusEnabled {
		return User{}, ErrUserDisabled
	}
	return user, nil
}
