package controllers

import (
	"testing"

	"flow-talk/models"
)

func TestPublishMessageDeliverUsesRealtimeBus(t *testing.T) {
	bus := models.NewMemoryRealtimeBus()
	hub := models.NewWSHub()
	delivered := make(chan models.MessageDeliverEvent, 1)

	if err := bus.SubscribeMessageDeliver(func(event models.MessageDeliverEvent) {
		delivered <- event
	}); err != nil {
		t.Fatalf("SubscribeMessageDeliver() error = %v", err)
	}

	event := models.MessageDeliverEvent{
		UserIDs: []int64{1, 2},
		Message: models.MessageDTO{
			ID:             10,
			ConversationID: 20,
			SenderID:       1,
		},
	}

	if err := publishMessageDeliver(bus, hub, event); err != nil {
		t.Fatalf("publishMessageDeliver() error = %v", err)
	}

	got := <-delivered
	if got.Message.ID != event.Message.ID {
		t.Fatalf("Message.ID = %d, want %d", got.Message.ID, event.Message.ID)
	}
}
