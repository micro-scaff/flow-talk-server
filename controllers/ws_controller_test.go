package controllers

import (
	"encoding/json"
	"testing"

	"flow-talk/models"
)

func TestDeliverConversationChangedToLocalHubTargetsRecipients(t *testing.T) {
	hub := models.NewWSHub()
	recipient := models.NewWSConnection(8, "recipient-device")
	nonRecipient := models.NewWSConnection(9, "other-device")
	hub.Add(recipient)
	hub.Add(nonRecipient)

	DeliverConversationChangedToLocalHub(hub, models.ConversationChangedEvent{
		UserIDs: []int64{8},
		Change: models.ConversationChangedPayload{
			ConversationID: 16,
			ChangeType:     models.ConversationChangeProfile,
		},
	})

	select {
	case payload := <-recipient.Send:
		var event models.WSEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if event.Type != models.WSEventConversationChanged {
			t.Fatalf("event.Type = %q, want %q", event.Type, models.WSEventConversationChanged)
		}
		var change models.ConversationChangedPayload
		if err := json.Unmarshal(event.Payload, &change); err != nil {
			t.Fatalf("json.Unmarshal(event.Payload) error = %v", err)
		}
		if change.ConversationID != 16 || change.ChangeType != models.ConversationChangeProfile {
			t.Fatalf("event change = %#v", change)
		}
	default:
		t.Fatal("recipient did not receive conversation.changed")
	}

	select {
	case payload := <-nonRecipient.Send:
		t.Fatalf("non-recipient received payload %s", payload)
	default:
	}
}
