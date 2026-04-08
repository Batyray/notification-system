package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/batyray/notification-system/pkg/logger"
	"github.com/batyray/notification-system/pkg/models"
	"github.com/batyray/notification-system/pkg/tasks"
	"github.com/batyray/notification-system/services/worker/delivery"
	"github.com/batyray/notification-system/services/worker/ratelimit"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// Create table manually since SQLite doesn't support gen_random_uuid()
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

func setupTestLogger() *logger.Logger {
	return logger.New(logger.Options{
		Service:     "worker-test",
		Environment: "test",
		Version:     "0.0.1",
		Writer:      io.Discard,
	})
}

func createTestTask(t *testing.T, notificationID uuid.UUID, channel, priority string) *asynq.Task {
	t.Helper()
	payload := tasks.NotificationPayload{
		NotificationID: notificationID,
		Channel:        channel,
		Priority:       priority,
		CorrelationID:  "test-corr-id",
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return asynq.NewTask(tasks.TypeNotificationSMS, data)
}

func TestHandleNotification_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(delivery.SendResponse{
			MessageID: "provider-msg-123",
			Status:    "accepted",
			Timestamp: "2026-03-31T12:00:00Z",
		})
	}))
	defer server.Close()

	db := setupTestDB(t)
	notificationID := uuid.New()
	notification := models.Notification{
		ID:        notificationID,
		Recipient: "+1234567890",
		Channel:   models.ChannelSMS,
		Content:   "Hello World",
		Priority:  models.PriorityNormal,
		Status:    models.StatusPending,
	}
	require.NoError(t, db.Create(&notification).Error)

	h := &Handler{
		DB:             db,
		DeliveryClient: delivery.NewClient(server.URL),
		Logger:         setupTestLogger(),
	}

	task := createTestTask(t, notificationID, "sms", "normal")
	err := h.HandleNotification(context.Background(), task)
	require.NoError(t, err)

	var updated models.Notification
	require.NoError(t, db.First(&updated, "id = ?", notificationID).Error)
	assert.Equal(t, models.StatusSent, updated.Status)
	assert.NotNil(t, updated.ProviderMessageID)
	assert.Equal(t, "provider-msg-123", *updated.ProviderMessageID)
	assert.NotNil(t, updated.SentAt)
	assert.Equal(t, 1, updated.RetryCount)
}

func TestHandleNotification_TransientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	db := setupTestDB(t)
	notificationID := uuid.New()
	notification := models.Notification{
		ID:        notificationID,
		Recipient: "+1234567890",
		Channel:   models.ChannelSMS,
		Content:   "Hello World",
		Priority:  models.PriorityNormal,
		Status:    models.StatusPending,
	}
	require.NoError(t, db.Create(&notification).Error)

	h := &Handler{
		DB:             db,
		DeliveryClient: delivery.NewClient(server.URL),
		Logger:         setupTestLogger(),
	}

	task := createTestTask(t, notificationID, "sms", "normal")
	err := h.HandleNotification(context.Background(), task)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, asynq.SkipRetry)

	var updated models.Notification
	require.NoError(t, db.First(&updated, "id = ?", notificationID).Error)
	assert.Equal(t, models.StatusFailed, updated.Status)
	assert.NotNil(t, updated.ErrorMessage)
}

func TestHandleNotification_PermanentError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	db := setupTestDB(t)
	notificationID := uuid.New()
	notification := models.Notification{
		ID:        notificationID,
		Recipient: "+1234567890",
		Channel:   models.ChannelSMS,
		Content:   "Hello World",
		Priority:  models.PriorityNormal,
		Status:    models.StatusPending,
	}
	require.NoError(t, db.Create(&notification).Error)

	h := &Handler{
		DB:             db,
		DeliveryClient: delivery.NewClient(server.URL),
		Logger:         setupTestLogger(),
	}

	task := createTestTask(t, notificationID, "sms", "normal")
	err := h.HandleNotification(context.Background(), task)
	assert.ErrorIs(t, err, asynq.SkipRetry)

	var updated models.Notification
	require.NoError(t, db.First(&updated, "id = ?", notificationID).Error)
	assert.Equal(t, models.StatusFailed, updated.Status)
	assert.NotNil(t, updated.ErrorMessage)
}

func TestHandleNotification_WithTemplate(t *testing.T) {
	var receivedContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		receivedContent = body["content"]

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(delivery.SendResponse{
			MessageID: "msg-tmpl-123",
			Status:    "accepted",
			Timestamp: "2026-03-31T12:00:00Z",
		})
	}))
	defer server.Close()

	db := setupTestDB(t)
	notificationID := uuid.New()
	templateVars := `{"name":"Alice"}`
	notification := models.Notification{
		ID:           notificationID,
		Recipient:    "+1234567890",
		Channel:      models.ChannelSMS,
		Content:      "Hello {{.name}}, welcome!",
		Priority:     models.PriorityNormal,
		Status:       models.StatusPending,
		TemplateVars: &templateVars,
	}
	require.NoError(t, db.Create(&notification).Error)

	h := &Handler{
		DB:             db,
		DeliveryClient: delivery.NewClient(server.URL),
		Logger:         setupTestLogger(),
	}

	task := createTestTask(t, notificationID, "sms", "normal")
	err := h.HandleNotification(context.Background(), task)
	require.NoError(t, err)

	assert.Equal(t, "Hello Alice, welcome!", receivedContent)

	var updated models.Notification
	require.NoError(t, db.First(&updated, "id = ?", notificationID).Error)
	assert.Equal(t, models.StatusSent, updated.Status)
}

func TestHandleNotification_CancelledSkipped(t *testing.T) {
	var serverCalled atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled.Store(true)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	db := setupTestDB(t)
	notificationID := uuid.New()
	notification := models.Notification{
		ID:        notificationID,
		Recipient: "+1234567890",
		Channel:   models.ChannelSMS,
		Content:   "Hello World",
		Priority:  models.PriorityNormal,
		Status:    models.StatusCancelled,
	}
	require.NoError(t, db.Create(&notification).Error)

	h := &Handler{
		DB:             db,
		DeliveryClient: delivery.NewClient(server.URL),
		Logger:         setupTestLogger(),
	}

	task := createTestTask(t, notificationID, "sms", "normal")
	err := h.HandleNotification(context.Background(), task)
	require.NoError(t, err)

	assert.False(t, serverCalled.Load(), "delivery server should not have been called for cancelled notification")
}

func TestHandleNotification_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("delivery should not be called when rate limited")
	}))
	defer server.Close()

	db := setupTestDB(t)
	notificationID := uuid.New()
	notification := models.Notification{
		ID:        notificationID,
		Recipient: "+1234567890",
		Channel:   models.ChannelSMS,
		Content:   "Hello World",
		Priority:  models.PriorityNormal,
		Status:    models.StatusPending,
	}
	require.NoError(t, db.Create(&notification).Error)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	limiter := ratelimit.New(rdb, 1, 1*time.Second)

	// Use up the rate limit
	allowed, err := limiter.Allow(context.Background(), "sms")
	require.NoError(t, err)
	require.True(t, allowed)

	h := &Handler{
		DB:             db,
		DeliveryClient: delivery.NewClient(server.URL),
		Logger:         setupTestLogger(),
		Limiter:        limiter,
	}

	task := createTestTask(t, notificationID, "sms", "normal")
	err = h.HandleNotification(context.Background(), task)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, asynq.SkipRetry)

	var rateLimitErr *ratelimit.RateLimitedError
	assert.ErrorAs(t, err, &rateLimitErr)

	// Notification should still be pending (not moved to processing)
	var updated models.Notification
	require.NoError(t, db.First(&updated, "id = ?", notificationID).Error)
	assert.Equal(t, models.StatusPending, updated.Status)
}
