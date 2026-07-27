package models

import (
	"reflect"
	"testing"
)

func TestMemoryRealtimeBusPublishesConversationChanged(t *testing.T) {
	bus := NewMemoryRealtimeBus()
	want := ConversationChangedEvent{
		UserIDs: []int64{1, 2, 3},
		Change: ConversationChangedPayload{
			ConversationID: 16,
			ChangeType:     ConversationChangeMembers,
		},
	}

	var received ConversationChangedEvent
	if err := bus.SubscribeConversationChanged(func(event ConversationChangedEvent) {
		received = event
	}); err != nil {
		t.Fatalf("SubscribeConversationChanged() error = %v", err)
	}
	if err := bus.PublishConversationChanged(want); err != nil {
		t.Fatalf("PublishConversationChanged() error = %v", err)
	}

	if !reflect.DeepEqual(received, want) {
		t.Fatalf("received event = %#v, want %#v", received, want)
	}
}
