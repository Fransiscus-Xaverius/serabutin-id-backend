package chatclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ticket 8: service-to-service chat directory proxy — metadata only.

func TestClient_ListConversations_SendsFilterAndToken(t *testing.T) {
	t.Parallel()
	var gotPath, gotQuery, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotToken = r.URL.Path, r.URL.Query().Encode(), r.Header.Get("X-Service-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"conversation_id":"c-1","user_a_id":5,"user_b_id":9,"message_count":7,"last_activity_at":"2026-10-14T12:00:00Z"}],"pagination":{"page":1,"limit":20,"total":1}}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "svc-secret", 5*time.Second)
	rows, total, err := c.ListConversations(context.Background(), 5, 1, 20)

	require.NoError(t, err)
	assert.Equal(t, "/api/v1/admin/chats", gotPath)
	assert.Contains(t, gotQuery, "user_id=5")
	assert.Contains(t, gotQuery, "page=1")
	assert.Contains(t, gotQuery, "limit=20")
	assert.Equal(t, "svc-secret", gotToken)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, "c-1", rows[0].ConversationID)
	assert.Equal(t, uint(5), rows[0].UserAID)
	assert.Equal(t, int64(7), rows[0].MessageCount)
}

func TestClient_ListConversations_UnconfiguredBaseURL_Errors(t *testing.T) {
	t.Parallel()
	c := NewClient("", "svc-secret", 5*time.Second)

	_, _, err := c.ListConversations(context.Background(), 0, 1, 20)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestClient_ListConversations_Non200_Errors(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "wrong", 5*time.Second)
	_, _, err := c.ListConversations(context.Background(), 0, 1, 20)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}
