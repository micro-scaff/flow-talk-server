package middlewares

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLimitAPIRequestBodyWithPathLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(LimitAPIRequestBodyWithPathLimits(
		DefaultMaxAPIRequestBodyBytes,
		map[string]int64{
			"/api/auth/register": MaxRegisterRequestBodyBytes,
		},
	))
	router.POST("/api/auth/register", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.POST("/api/messages", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	body := bytes.Repeat([]byte("a"), int(DefaultMaxAPIRequestBodyBytes+1))

	t.Run("allows larger registration payload", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
		}
	})

	t.Run("keeps default limit for other APIs", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
		}
	})
}
