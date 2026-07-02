package models

import (
	"errors"
	"strings"
)

var ErrUnsupportedAuthProvider = errors.New("不支持的外部身份提供方")

// ExternalUserProfile 是外部身份系统返回给 IM 服务的用户资料。
// IM 只信任 external_id 作为稳定映射键，昵称和头像可以随外部系统变化同步更新。
type ExternalUserProfile struct {
	ExternalID string
	Username   string
	Nickname   string
	AvatarURL  string
}

// TokenVerifier 抽象外部 access_token 校验。
// 当前实现只有 DemoTokenVerifier，后续接企业 SSO、OAuth 或网关时只需要新增 verifier。
type TokenVerifier interface {
	Verify(accessToken string) (ExternalUserProfile, error)
}

// AuthProvider 把 provider 名称和 verifier 绑定起来。
type AuthProvider struct {
	Name     string
	Verifier TokenVerifier
}

// DemoTokenVerifier 是本地开发用的外部身份模拟器。
// 规则：access_token 非空即可通过；token 本身会成为 external_id 的一部分。
type DemoTokenVerifier struct{}

func (DemoTokenVerifier) Verify(accessToken string) (ExternalUserProfile, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return ExternalUserProfile{}, ErrValidation
	}

	externalID := "demo:" + accessToken
	username := "demo_" + strings.NewReplacer(":", "_", " ", "_", "@", "_").Replace(accessToken)
	return ExternalUserProfile{
		ExternalID: externalID,
		Username:   username,
		Nickname:   username,
	}, nil
}

func verifierForProvider(provider string) (TokenVerifier, error) {
	switch strings.TrimSpace(provider) {
	case "demo":
		return DemoTokenVerifier{}, nil
	default:
		return nil, ErrUnsupportedAuthProvider
	}
}
