package models

// PresenceDTO 是用户在线状态的接口输出。
// Online/ConnectionCount 来自当前进程内存 Hub，LastSeenAt 兼容设备表的离线最近活跃时间。
type PresenceDTO struct {
	UserID          int64  `json:"user_id"`
	Online          bool   `json:"online"`
	ConnectionCount int    `json:"connection_count"`
	LastActiveAt    string `json:"last_active_at,omitempty"`
	LastSeenAt      string `json:"last_seen_at,omitempty"`
}

// PresenceProvider 抽象在线状态来源。默认实现读取本机 Hub；启用 Redis 后读取全局 presence。
type PresenceProvider interface {
	Presence(userID int64) (PresenceDTO, error)
	BatchPresence(userIDs []int64) ([]PresenceDTO, error)
}

// HubPresenceProvider 从本机 WSHub 读取在线状态，适用于单实例部署和本地开发。
type HubPresenceProvider struct {
	Hub *WSHub
}

// NewHubPresenceProvider 创建基于本机 Hub 的在线状态读取器。
func NewHubPresenceProvider(hub *WSHub) HubPresenceProvider {
	return HubPresenceProvider{Hub: hub}
}

// Presence 查询单个用户在当前进程内的在线状态。
// Hub 为空时返回离线状态，方便测试或未初始化实时模块时安全降级。
func (p HubPresenceProvider) Presence(userID int64) (PresenceDTO, error) {
	if p.Hub == nil {
		return PresenceDTO{UserID: userID}, nil
	}
	return p.Hub.LocalPresence(userID), nil
}

// BatchPresence 批量查询本机 Hub 中的在线状态。
// 这里会复用 uniquePositiveIDs 去重和排序，避免同一个用户重复出现在响应里。
func (p HubPresenceProvider) BatchPresence(userIDs []int64) ([]PresenceDTO, error) {
	userIDs = uniquePositiveIDs(userIDs)
	if len(userIDs) == 0 {
		return nil, ErrInvalidMember
	}

	result := make([]PresenceDTO, 0, len(userIDs))
	for _, userID := range userIDs {
		presence, err := p.Presence(userID)
		if err != nil {
			return nil, err
		}
		result = append(result, presence)
	}
	return result, nil
}

// GetUserPresence 返回单个用户的在线状态。
func GetUserPresence(provider PresenceProvider, userID int64) (PresenceDTO, error) {
	if userID <= 0 {
		return PresenceDTO{}, ErrInvalidMember
	}

	if provider == nil {
		provider = NewHubPresenceProvider(nil)
	}

	// 在线状态由注入的 provider 决定：默认本机 Hub，启用 Redis 后为全局 presence。
	presence, err := provider.Presence(userID)
	if err != nil {
		return PresenceDTO{}, err
	}
	if DB == nil {
		return presence, nil
	}
	// last_seen_at 取自设备表 updated_at，即使用户当前离线，也能给客户端展示最近活跃时间。
	seenAt, err := LatestDeviceSeenAt(userID)
	if err != nil {
		return PresenceDTO{}, err
	}
	presence.LastSeenAt = timeString(seenAt)
	return presence, nil
}

// BatchUserPresence 批量查询用户在线状态。
func BatchUserPresence(provider PresenceProvider, userIDs []int64) ([]PresenceDTO, error) {
	// 去重和排序放在 model 层，保证不同 controller/调用方拿到一致的结果顺序。
	userIDs = uniquePositiveIDs(userIDs)
	if len(userIDs) == 0 {
		return nil, ErrInvalidMember
	}
	if provider == nil {
		provider = NewHubPresenceProvider(nil)
	}

	// 当前数据量较小，逐个查询可读性更好。
	// 后续如果需要大批量在线状态，可以把 LatestDeviceSeenAt 改成批量 SQL。
	result := make([]PresenceDTO, 0, len(userIDs))
	for _, userID := range userIDs {
		presence, err := GetUserPresence(provider, userID)
		if err != nil {
			return nil, err
		}
		result = append(result, presence)
	}
	return result, nil
}
