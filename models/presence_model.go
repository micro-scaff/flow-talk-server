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

// GetUserPresence 返回单个用户的在线状态。
func GetUserPresence(hub *WSHub, userID int64) (PresenceDTO, error) {
	if userID <= 0 {
		return PresenceDTO{}, ErrInvalidMember
	}

	// 在线状态优先取 Hub 的本机连接快照。
	// 这是实时值，但只代表当前进程；多实例部署时应由 Redis presence 或 RealtimeBus 聚合。
	presence := hub.LocalPresence(userID)
	// last_seen_at 来自设备表，即使用户当前离线，也能给客户端展示最近活跃时间。
	seenAt, err := LatestDeviceSeenAt(userID)
	if err != nil {
		return PresenceDTO{}, err
	}
	presence.LastSeenAt = timeString(seenAt)
	return presence, nil
}

// BatchUserPresence 批量查询用户在线状态。
func BatchUserPresence(hub *WSHub, userIDs []int64) ([]PresenceDTO, error) {
	// 去重和排序放在 model 层，保证不同 controller/调用方拿到一致的结果顺序。
	userIDs = uniquePositiveIDs(userIDs)
	if len(userIDs) == 0 {
		return nil, ErrInvalidMember
	}

	// 当前数据量较小，逐个查询可读性更好。
	// 后续如果需要大批量在线状态，可以把 LatestDeviceSeenAt 改成批量 SQL。
	result := make([]PresenceDTO, 0, len(userIDs))
	for _, userID := range userIDs {
		presence, err := GetUserPresence(hub, userID)
		if err != nil {
			return nil, err
		}
		result = append(result, presence)
	}
	return result, nil
}
