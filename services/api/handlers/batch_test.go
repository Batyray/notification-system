package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateBatch_Success(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"notifications": [
		{"recipient": "+111", "channel": "sms", "content": "Hello 1"},
		{"recipient": "+222", "channel": "sms", "content": "Hello 2"},
		{"recipient": "a@b.com", "channel": "email", "content": "Hello 3"}
	]}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateBatch(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp["batch_id"])
	ids := resp["notification_ids"].([]interface{})
	assert.Len(t, ids, 3)
}

func TestCreateBatch_ExceedsLimit(t *testing.T) {
	h, _ := setupTestHandler(t)

	notifications := make([]map[string]string, 1001)
	for i := range notifications {
		notifications[i] = map[string]string{"recipient": "+111", "channel": "sms", "content": "msg"}
	}
	body, _ := json.Marshal(map[string]interface{}{"notifications": notifications})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateBatch(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateBatch_InvalidNotification(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"notifications": [{"recipient": "+111", "channel": "invalid", "content": "Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateBatch(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetBatchNotifications_Success(t *testing.T) {
	h, _ := setupTestHandler(t)

	// Create batch first
	body := `{"notifications": [
		{"recipient": "+111", "channel": "sms", "content": "Hello 1"},
		{"recipient": "+222", "channel": "sms", "content": "Hello 2"}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateBatch(w, req)

	var createResp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &createResp)
	assert.NoError(t, err)
	batchID := createResp["batch_id"].(string)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("batchId", batchID)
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), chi.RouteCtxKey, rctx))
	getW := httptest.NewRecorder()
	h.GetBatchNotifications(getW, getReq)

	assert.Equal(t, http.StatusOK, getW.Code)
	var resp map[string]interface{}
	err = json.Unmarshal(getW.Body.Bytes(), &resp)
	assert.NoError(t, err)
	data := resp["data"].([]interface{})
	assert.Len(t, data, 2)
}
