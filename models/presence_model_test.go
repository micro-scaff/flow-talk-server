package models

import "testing"

type fakePresenceProvider struct {
	presence PresenceDTO
}

func (p fakePresenceProvider) Presence(userID int64) (PresenceDTO, error) {
	presence := p.presence
	presence.UserID = userID
	return presence, nil
}

func (p fakePresenceProvider) BatchPresence(userIDs []int64) ([]PresenceDTO, error) {
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

func TestGetUserPresenceUsesInjectedProvider(t *testing.T) {
	provider := fakePresenceProvider{
		presence: PresenceDTO{
			Online:          true,
			ConnectionCount: 2,
			LastActiveAt:    "2026-06-29T12:00:00+08:00",
		},
	}

	presence, err := GetUserPresence(provider, 42)
	if err != nil {
		t.Fatalf("GetUserPresence() error = %v", err)
	}

	if presence.UserID != 42 {
		t.Fatalf("UserID = %d", presence.UserID)
	}
	if !presence.Online {
		t.Fatalf("Online = false, want true")
	}
	if presence.ConnectionCount != 2 {
		t.Fatalf("ConnectionCount = %d", presence.ConnectionCount)
	}
	if presence.LastActiveAt == "" {
		t.Fatalf("LastActiveAt is empty")
	}
}
