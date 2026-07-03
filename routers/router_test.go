package routers

import (
	"net/http"
	"testing"
	"time"

	"flow-talk/models"

	"github.com/gin-gonic/gin"
)

func TestInitRouterUsesBodyBasedAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	InitRouter(engine, models.AppConfig{
		JWT: models.JWTConfig{
			Secret: "test-secret",
			TTL:    time.Hour,
		},
	})

	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	expected := []string{
		http.MethodGet + " /api/conversations",
		http.MethodPost + " /api/conversations/detail",
		http.MethodPost + " /api/conversations/direct",
		http.MethodPost + " /api/conversations/groups",
		http.MethodPatch + " /api/conversations/profile",
		http.MethodPost + " /api/conversations/members",
		http.MethodDelete + " /api/conversations/members",
		http.MethodPost + " /api/conversations/leave",
		http.MethodPatch + " /api/conversations/members/role",
		http.MethodPost + " /api/conversations/messages",
		http.MethodPost + " /api/conversations/messages/list",
		http.MethodPost + " /api/conversations/messages/search",
		http.MethodPost + " /api/conversations/read",
		http.MethodPost + " /api/messages/search",
		http.MethodPost + " /api/users/presence",
		http.MethodPost + " /api/users/presence/batch",
		http.MethodPost + " /api/messages/receipts",
		http.MethodPost + " /api/messages/read",
		http.MethodPost + " /api/messages/unread",
	}
	for _, route := range expected {
		if !routes[route] {
			t.Fatalf("missing route %s", route)
		}
	}

	removed := []string{
		http.MethodGet + " /api/conversations/:conversation_id",
		http.MethodPatch + " /api/conversations/:conversation_id",
		http.MethodPost + " /api/conversations/:conversation_id/members",
		http.MethodDelete + " /api/conversations/:conversation_id/members/:user_id",
		http.MethodPost + " /api/conversations/:conversation_id/leave",
		http.MethodPatch + " /api/conversations/:conversation_id/members/:user_id/role",
		http.MethodPost + " /api/conversations/:conversation_id/messages",
		http.MethodGet + " /api/conversations/:conversation_id/messages",
		http.MethodGet + " /api/conversations/:conversation_id/messages/search",
		http.MethodPost + " /api/conversations/:conversation_id/read",
		http.MethodGet + " /api/messages/search",
		http.MethodPatch + " /api/messages/:message_id/recall",
		http.MethodPatch + " /api/messages/:message_id/delete",
		http.MethodGet + " /api/messages/:message_id/receipts",
		http.MethodPost + " /api/messages/:message_id/delivered",
		http.MethodPost + " /api/messages/:message_id/read",
		http.MethodGet + " /api/users/:user_id/presence",
	}
	for _, route := range removed {
		if routes[route] {
			t.Fatalf("old route still registered: %s", route)
		}
	}
}
