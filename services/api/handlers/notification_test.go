package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/batyray/notification-system/pkg/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Create table manually to avoid SQLite issues with PostgreSQL-specific
	// types like "uuid" and defaults like "gen_random_uuid()".
	err = db.Exec(`CREATE TABLE notifications (
		id TEXT PRIMARY KEY,
		batch_id TEXT,
		idempotency_key TEXT UNIQUE,
		recipient TEXT NOT NULL,
		channel TEXT NOT NULL,
		content TEXT NOT NULL,
		priority TEXT NOT NULL DEFAULT 'normal',
		status TEXT NOT NULL DEFAULT 'pending',
		provider_message_id TEXT,
		retry_count INTEGER DEFAULT 0,
		error_message TEXT,
		correlation_id TEXT,
		template_vars TEXT,
		scheduled_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME,
		sent_at DATETIME
	)`).Error
	require.NoError(t, err)

	return db
}

func setupTestHandler(t *testing.T) (*Handler, *gorm.DB) {
	t.Helper()
	db := setupTestDB(t)
	h := &Handler{
		DB: db,
	}
	return h, db
}

// withChiURLParam adds a chi URL parameter to the request context.
func withChiURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestCreateNotification_Success(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := CreateNotificationRequest{
		Recipient: "+1234567890",
		Channel:   "sms",
		Content:   "Hello, world!",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateNotification(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp models.Notification
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "+1234567890", resp.Recipient)
	assert.Equal(t, models.ChannelSMS, resp.Channel)
	assert.Equal(t, "Hello, world!", resp.Content)
	assert.Equal(t, models.StatusPending, resp.Status)
	assert.Equal(t, models.PriorityNormal, resp.Priority)
	assert.NotEqual(t, uuid.Nil, resp.ID)
}

func TestCreateNotification_InvalidChannel(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := CreateNotificationRequest{
		Recipient: "+1234567890",
		Channel:   "telegram",
		Content:   "Hello",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateNotification(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "invalid channel")
}

func TestCreateNotification_MissingRecipient(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := CreateNotificationRequest{
		Channel: "sms",
		Content: "Hello",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateNotification(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "recipient is required")
}

func TestCreateNotification_MissingContent(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := CreateNotificationRequest{
		Recipient: "+1234567890",
		Channel:   "sms",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateNotification(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "content is required")
}

func TestCreateNotification_ScheduledAt(t *testing.T) {
	h, _ := setupTestHandler(t)

	scheduledAt := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)
	body := CreateNotificationRequest{
		Recipient:   "+1234567890",
		Channel:     "email",
		Content:     "Scheduled message",
		ScheduledAt: &scheduledAt,
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateNotification(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp models.Notification
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, models.StatusPending, resp.Status)
	require.NotNil(t, resp.ScheduledAt)
	assert.Equal(t, scheduledAt.Unix(), resp.ScheduledAt.Unix())
}

func TestGetNotification_Success(t *testing.T) {
	h, db := setupTestHandler(t)

	// Create a notification directly in the DB
	n := models.Notification{
		Recipient:     "+1234567890",
		Channel:       models.ChannelSMS,
		Content:       "Test message",
		Priority:      models.PriorityNormal,
		Status:        models.StatusPending,
		CorrelationID: uuid.New().String(),
	}
	err := db.Create(&n).Error
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications/"+n.ID.String(), nil)
	req = withChiURLParam(req, "id", n.ID.String())
	w := httptest.NewRecorder()

	h.GetNotification(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.Notification
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, n.ID, resp.ID)
	assert.Equal(t, n.Recipient, resp.Recipient)
}

func TestGetNotification_NotFound(t *testing.T) {
	h, _ := setupTestHandler(t)

	randomID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/v1/notifications/"+randomID, nil)
	req = withChiURLParam(req, "id", randomID)
	w := httptest.NewRecorder()

	h.GetNotification(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "not found")
}

func TestCancelNotification_Success(t *testing.T) {
	h, db := setupTestHandler(t)

	n := models.Notification{
		Recipient:     "+1234567890",
		Channel:       models.ChannelSMS,
		Content:       "Test",
		Priority:      models.PriorityNormal,
		Status:        models.StatusPending,
		CorrelationID: uuid.New().String(),
	}
	err := db.Create(&n).Error
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/"+n.ID.String()+"/cancel", nil)
	req = withChiURLParam(req, "id", n.ID.String())
	w := httptest.NewRecorder()

	h.CancelNotification(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.Notification
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, models.StatusCancelled, resp.Status)
}

func TestCancelNotification_NotPending(t *testing.T) {
	h, db := setupTestHandler(t)

	n := models.Notification{
		Recipient:     "+1234567890",
		Channel:       models.ChannelSMS,
		Content:       "Test",
		Priority:      models.PriorityNormal,
		Status:        models.StatusSent,
		CorrelationID: uuid.New().String(),
	}
	err := db.Create(&n).Error
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/"+n.ID.String()+"/cancel", nil)
	req = withChiURLParam(req, "id", n.ID.String())
	w := httptest.NewRecorder()

	h.CancelNotification(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "cannot cancel")
}

func TestListNotifications_Empty(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications", nil)
	w := httptest.NewRecorder()

	h.ListNotifications(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].([]interface{})
	require.True(t, ok)
	assert.Empty(t, data)
	assert.Nil(t, resp["next_cursor"])
}

func TestListNotifications_WithFilter(t *testing.T) {
	h, db := setupTestHandler(t)

	// Create a mix of notifications
	notifications := []models.Notification{
		{Recipient: "+1", Channel: models.ChannelSMS, Content: "sms1", Priority: models.PriorityNormal, Status: models.StatusPending, CorrelationID: uuid.New().String()},
		{Recipient: "+2", Channel: models.ChannelEmail, Content: "email1", Priority: models.PriorityNormal, Status: models.StatusPending, CorrelationID: uuid.New().String()},
		{Recipient: "+3", Channel: models.ChannelSMS, Content: "sms2", Priority: models.PriorityNormal, Status: models.StatusSent, CorrelationID: uuid.New().String()},
		{Recipient: "+4", Channel: models.ChannelEmail, Content: "email2", Priority: models.PriorityNormal, Status: models.StatusSent, CorrelationID: uuid.New().String()},
	}

	for i := range notifications {
		err := db.Create(&notifications[i]).Error
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond) // ensure different timestamps
	}

	// Filter: channel=sms, status=pending
	req := httptest.NewRequest(http.MethodGet, "/v1/notifications?channel=sms&status=pending", nil)
	w := httptest.NewRecorder()

	h.ListNotifications(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 1)

	first := data[0].(map[string]interface{})
	assert.Equal(t, "sms", first["channel"])
	assert.Equal(t, "pending", first["status"])
}

func TestListNotifications_CursorPagination(t *testing.T) {
	h, db := setupTestHandler(t)

	// Create 5 notifications with slightly different timestamps
	for i := 0; i < 5; i++ {
		n := models.Notification{
			Recipient:     fmt.Sprintf("+%d", i),
			Channel:       models.ChannelSMS,
			Content:       fmt.Sprintf("msg %d", i),
			Priority:      models.PriorityNormal,
			Status:        models.StatusPending,
			CorrelationID: uuid.New().String(),
		}
		err := db.Create(&n).Error
		require.NoError(t, err)
		time.Sleep(5 * time.Millisecond) // ensure different created_at
	}

	// Page 1: get first 2
	req := httptest.NewRequest(http.MethodGet, "/v1/notifications?page_size=2", nil)
	w := httptest.NewRecorder()
	h.ListNotifications(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var page1 map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &page1)
	require.NoError(t, err)

	data1, ok := page1["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data1, 2)
	require.NotNil(t, page1["next_cursor"])
	cursor1 := page1["next_cursor"].(string)

	// Page 2: use cursor
	req = httptest.NewRequest(http.MethodGet, "/v1/notifications?page_size=2&cursor="+cursor1, nil)
	w = httptest.NewRecorder()
	h.ListNotifications(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var page2 map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &page2)
	require.NoError(t, err)

	data2, ok := page2["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data2, 2)
	require.NotNil(t, page2["next_cursor"])
	cursor2 := page2["next_cursor"].(string)

	// Page 3: last page should have 1 item and no next_cursor
	req = httptest.NewRequest(http.MethodGet, "/v1/notifications?page_size=2&cursor="+cursor2, nil)
	w = httptest.NewRecorder()
	h.ListNotifications(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var page3 map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &page3)
	require.NoError(t, err)

	data3, ok := page3["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data3, 1)
	assert.Nil(t, page3["next_cursor"])

	// Verify no duplicates across pages
	allRecipients := make(map[string]bool)
	for _, page := range [][]interface{}{data1, data2, data3} {
		for _, item := range page {
			r := item.(map[string]interface{})["recipient"].(string)
			assert.False(t, allRecipients[r], "duplicate recipient found: %s", r)
			allRecipients[r] = true
		}
	}
	assert.Len(t, allRecipients, 5, "should have all 5 notifications across pages")
}
