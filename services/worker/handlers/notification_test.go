package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/batyray/notification-system/pkg/logger"
	"github.com/batyray/notification-system/pkg/models"
	"github.com/batyray/notification-system/pkg/tasks"
	"github.com/batyray/notification-system/services/worker/delivery"
	"github.com/batyray/notification-system/services/worker/ratelimit"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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
		TraceCarrier:   nil,
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return asynq.NewTask(tasks.TypeNotificationSMS, data)
}

func TestHandleNotification_CreatesTraceSpans(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)

	mock := &mockSender{
		SendFunc: func(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error) {
			return &delivery.SendResponse{
				MessageID: "msg-trace-123",
				Status:    "accepted",
				Timestamp: "2026-04-08T12:00:00Z",
			}, nil
		},
	}

	db := setupTestDB(t)
	notificationID := uuid.New()
	notification := models.Notification{
		ID:        notificationID,
		Recipient: "+1234567890",
		Channel:   models.ChannelSMS,
		Content:   "Hello Traced",
		Priority:  models.PriorityNormal,
		Status:    models.StatusPending,
	}
	require.NoError(t, db.Create(&notification).Error)

	h := &Handler{
		DB:             db,
		DeliveryClient: mock,
		Logger:         setupTestLogger(),
	}

	task := createTestTask(t, notificationID, "sms", "normal")
	err := h.HandleNotification(context.Background(), task)
	require.NoError(t, err)

	spans := exporter.GetSpans()
	spanNames := make([]string, len(spans))
	for i, s := range spans {
		spanNames[i] = s.Name
	}
	assert.Contains(t, spanNames, "worker.HandleNotification")
	assert.Contains(t, spanNames, "webhook.deliver")
}

func TestHandleNotification_Success(t *testing.T) {
	mock := &mockSender{
		SendFunc: func(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error) {
			return &delivery.SendResponse{
				MessageID: "provider-msg-123",
				Status:    "accepted",
				Timestamp: "2026-03-31T12:00:00Z",
			}, nil
		},
	}

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
		DeliveryClient: mock,
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

	require.Len(t, mock.Calls, 1)
	assert.Equal(t, "+1234567890", mock.Calls[0].To)
	assert.Equal(t, "sms", mock.Calls[0].Channel)
}

func TestHandleNotification_TransientError(t *testing.T) {
	mock := &mockSender{
		SendFunc: func(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error) {
			return nil, &delivery.SendError{StatusCode: http.StatusInternalServerError}
		},
	}

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
		DeliveryClient: mock,
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
	mock := &mockSender{
		SendFunc: func(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error) {
			return nil, &delivery.SendError{StatusCode: http.StatusBadRequest}
		},
	}

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
		DeliveryClient: mock,
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
	mock := &mockSender{
		SendFunc: func(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error) {
			return &delivery.SendResponse{
				MessageID: "msg-tmpl-123",
				Status:    "accepted",
				Timestamp: "2026-03-31T12:00:00Z",
			}, nil
		},
	}

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
		DeliveryClient: mock,
		Logger:         setupTestLogger(),
	}

	task := createTestTask(t, notificationID, "sms", "normal")
	err := h.HandleNotification(context.Background(), task)
	require.NoError(t, err)

	require.Len(t, mock.Calls, 1)
	assert.Equal(t, "Hello Alice, welcome!", mock.Calls[0].Content)

	var updated models.Notification
	require.NoError(t, db.First(&updated, "id = ?", notificationID).Error)
	assert.Equal(t, models.StatusSent, updated.Status)
}

func TestHandleNotification_CancelledSkipped(t *testing.T) {
	mock := &mockSender{
		SendFunc: func(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error) {
			t.Fatal("delivery should not have been called for cancelled notification")
			return nil, nil
		},
	}

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
		DeliveryClient: mock,
		Logger:         setupTestLogger(),
	}

	task := createTestTask(t, notificationID, "sms", "normal")
	err := h.HandleNotification(context.Background(), task)
	require.NoError(t, err)

	assert.Empty(t, mock.Calls)
}

func TestHandleNotification_RateLimited_WaitsAndProceeds(t *testing.T) {
	mock := &mockSender{
		SendFunc: func(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error) {
			return &delivery.SendResponse{
				MessageID: "msg-rate-limited",
				Status:    "accepted",
				Timestamp: "2026-04-08T12:00:00Z",
			}, nil
		},
	}

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
	limiter := ratelimit.New(rdb, 1, 150*time.Millisecond)

	allowed, err := limiter.Allow(context.Background(), "sms")
	require.NoError(t, err)
	require.True(t, allowed)

	h := &Handler{
		DB:             db,
		DeliveryClient: mock,
		Logger:         setupTestLogger(),
		Limiter:        limiter,
	}

	task := createTestTask(t, notificationID, "sms", "normal")
	err = h.HandleNotification(context.Background(), task)
	require.NoError(t, err)

	var updated models.Notification
	require.NoError(t, db.First(&updated, "id = ?", notificationID).Error)
	assert.Equal(t, models.StatusSent, updated.Status)
}

func TestHandleNotification_RateLimited_RespectsContextCancel(t *testing.T) {
	mock := &mockSender{
		SendFunc: func(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error) {
			t.Fatal("delivery should not have been called when rate limited and context cancelled")
			return nil, nil
		},
	}

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
	limiter := ratelimit.New(rdb, 1, 10*time.Second)

	allowed, err := limiter.Allow(context.Background(), "sms")
	require.NoError(t, err)
	require.True(t, allowed)

	h := &Handler{
		DB:             db,
		DeliveryClient: mock,
		Logger:         setupTestLogger(),
		Limiter:        limiter,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	task := createTestTask(t, notificationID, "sms", "normal")
	err = h.HandleNotification(ctx, task)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	var updated models.Notification
	require.NoError(t, db.First(&updated, "id = ?", notificationID).Error)
	assert.Equal(t, models.StatusPending, updated.Status)
}
