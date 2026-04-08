//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const apiBaseURL = "http://localhost:8080"

func TestFullNotificationFlow(t *testing.T) {
	resp, err := http.Get(apiBaseURL + "/health")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body := `{"recipient": "+1234567890", "channel": "sms", "content": "Integration test message", "priority": "high"}`
	resp, err = http.Post(apiBaseURL+"/api/v1/notifications", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createResp)
	notifID := createResp["id"].(string)
	assert.NotEmpty(t, notifID)

	resp, err = http.Get(fmt.Sprintf("%s/api/v1/notifications/%s", apiBaseURL, notifID))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	time.Sleep(3 * time.Second)

	resp, err = http.Get(fmt.Sprintf("%s/api/v1/notifications/%s", apiBaseURL, notifID))
	require.NoError(t, err)
	var getResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&getResp)
	status := getResp["status"].(string)
	assert.Contains(t, []string{"sent", "failed", "processing"}, status)

	resp, err = http.Get(apiBaseURL + "/api/v1/metrics")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get(apiBaseURL + "/api/v1/notifications?channel=sms")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestBatchFlow(t *testing.T) {
	body := `{"notifications": [
		{"recipient": "+111", "channel": "sms", "content": "Batch 1", "priority": "high"},
		{"recipient": "+222", "channel": "email", "content": "Batch 2", "priority": "normal"},
		{"recipient": "+333", "channel": "push", "content": "Batch 3", "priority": "low"}
	]}`
	resp, err := http.Post(apiBaseURL+"/api/v1/notifications/batch", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var batchResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&batchResp)
	batchID := batchResp["batch_id"].(string)

	resp, err = http.Get(fmt.Sprintf("%s/api/v1/notifications/batch/%s", apiBaseURL, batchID))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestCancelFlow(t *testing.T) {
	body := `{"recipient": "+999", "channel": "sms", "content": "Will cancel", "priority": "low"}`
	resp, err := http.Post(apiBaseURL+"/api/v1/notifications", "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)

	var createResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createResp)
	notifID := createResp["id"].(string)

	req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("%s/api/v1/notifications/%s/cancel", apiBaseURL, notifID), nil)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var cancelResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&cancelResp)
	assert.Equal(t, "cancelled", cancelResp["status"])
}

func TestIdempotency(t *testing.T) {
	body := `{"recipient": "+555", "channel": "sms", "content": "Idempotent test"}`

	req1, _ := http.NewRequest(http.MethodPost, apiBaseURL+"/api/v1/notifications", bytes.NewBufferString(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "test-idem-key-123")
	resp1, err := http.DefaultClient.Do(req1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp1.StatusCode)

	var resp1Body map[string]interface{}
	json.NewDecoder(resp1.Body).Decode(&resp1Body)

	req2, _ := http.NewRequest(http.MethodPost, apiBaseURL+"/api/v1/notifications", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "test-idem-key-123")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp2.StatusCode)

	var resp2Body map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&resp2Body)
	assert.Equal(t, resp1Body["id"], resp2Body["id"])
}
