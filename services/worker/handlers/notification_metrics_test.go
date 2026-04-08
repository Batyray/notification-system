package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/batyray/notification-system/pkg/metrics"
	"github.com/batyray/notification-system/pkg/models"
	"github.com/batyray/notification-system/services/worker/delivery"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestHandleNotification_RecordsMetrics(t *testing.T) {
	// Init registers the OTel MeterProvider with a Prometheus exporter and
	// returns an http.Handler that can be scraped for metrics.
	metricsHandler, _, err := metrics.Init("test-worker")
	require.NoError(t, err)

	mock := &mockSender{
		SendFunc: func(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error) {
			return &delivery.SendResponse{
				MessageID: "msg-metrics-123",
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
		Content:   "Hello Metrics",
		Priority:  models.PriorityNormal,
		Status:    models.StatusPending,
	}
	require.NoError(t, db.Create(&notification).Error)

	h := &Handler{
		DB:             db,
		DeliveryClient: mock,
		Logger:         setupTestLogger(),
		Meter:          otel.Meter("test-worker"),
	}
	h.InitMetrics()

	asynqTask := createTestTask(t, notificationID, "sms", "normal")

	err = h.HandleNotification(context.Background(), asynqTask)
	require.NoError(t, err)

	// Scrape the Prometheus metrics endpoint.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsHandler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	metricsText := string(body)

	assert.True(t, strings.Contains(metricsText, "worker_tasks_processed_total"),
		"expected worker_tasks_processed_total in metrics output")
	assert.True(t, strings.Contains(metricsText, "worker_task_duration_seconds"),
		"expected worker_task_duration_seconds in metrics output")
}
