package models

import (
	"errors"
	"strings"
)

// ErrUnsupportedAuthProvider 表示客户端传入的 provider 当前没有对应校验器。
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
	// demo provider 只用于本地开发联调，不访问真实第三方系统。
	// 真实 provider 应在这里调用外部服务校验 access_token，并返回可信的 external_id。
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return ExternalUserProfile{}, ErrValidation
	}

	// external_id 加 provider 前缀，避免多个身份源出现相同用户 ID 时互相冲突。
	externalID := "demo:" + accessToken
	// username 需要落到 users.username 唯一列，因此把不适合作账号展示的字符替换掉。
	username := "demo_" + strings.NewReplacer(":", "_", " ", "_", "@", "_").Replace(accessToken)
	return ExternalUserProfile{
		ExternalID: externalID,
		Username:   username,
		Nickname:   username,
	}, nil
}

func verifierForProvider(provider string) (TokenVerifier, error) {
	// provider 路由集中放在这里。
	// 后续新增企业微信、OAuth、内部网关等 provider 时，只扩展这个 switch 或替换成注册表即可。
	switch strings.TrimSpace(provider) {
	case "demo":
		return DemoTokenVerifier{}, nil
	default:
		return nil, ErrUnsupportedAuthProvider
	}
}
