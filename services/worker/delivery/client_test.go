package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Send_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "+1234567890", body["to"])
		assert.Equal(t, "sms", body["channel"])
		assert.Equal(t, "Hello", body["content"])

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(SendResponse{
			MessageID: "msg-123",
			Status:    "accepted",
			Timestamp: "2026-03-31T12:00:00Z",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	resp, err := client.Send(context.Background(), SendRequest{
		To: "+1234567890", Channel: "sms", Content: "Hello", CorrelationID: "corr-123",
	})

	require.NoError(t, err)
	assert.Equal(t, "msg-123", resp.MessageID)
}

func TestClient_Send_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.Send(context.Background(), SendRequest{To: "+1234567890", Channel: "sms", Content: "Hello"})

	assert.Error(t, err)
	var se *SendError
	assert.ErrorAs(t, err, &se)
	assert.Equal(t, 500, se.StatusCode)
	assert.True(t, se.IsTransient())
}

func TestClient_Send_BadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.Send(context.Background(), SendRequest{To: "+1234567890", Channel: "sms", Content: "Hello"})

	var se *SendError
	assert.ErrorAs(t, err, &se)
	assert.Equal(t, 400, se.StatusCode)
	assert.False(t, se.IsTransient())
}

func TestClient_Send_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.Send(context.Background(), SendRequest{To: "+1234567890", Channel: "sms", Content: "Hello"})

	var se *SendError
	assert.ErrorAs(t, err, &se)
	assert.True(t, se.IsTransient())
	assert.True(t, se.IsRateLimited())
}
