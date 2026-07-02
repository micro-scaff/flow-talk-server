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

	presence := hub.LocalPresence(userID)
	seenAt, err := LatestDeviceSeenAt(userID)
	if err != nil {
		return PresenceDTO{}, err
	}
	presence.LastSeenAt = timeString(seenAt)
	return presence, nil
}

// BatchUserPresence 批量查询用户在线状态。
func BatchUserPresence(hub *WSHub, userIDs []int64) ([]PresenceDTO, error) {
	userIDs = uniquePositiveIDs(userIDs)
	if len(userIDs) == 0 {
		return nil, ErrInvalidMember
	}

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
