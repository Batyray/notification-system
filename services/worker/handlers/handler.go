package handlers

import (
	"github.com/batyray/notification-system/pkg/logger"
	"github.com/batyray/notification-system/services/worker/ratelimit"
	"go.opentelemetry.io/otel/metric"
	"gorm.io/gorm"
)

type Handler struct {
	DB             *gorm.DB
	DeliveryClient Sender
	Logger         *logger.Logger
	Limiter        *ratelimit.Limiter
	Meter          metric.Meter

	// Metrics instruments (created once via InitMetrics)
	taskDuration metric.Float64Histogram
	taskCount    metric.Int64Counter
	activeTasks  metric.Int64UpDownCounter
}

// InitMetrics creates the OTel instruments for task metrics.
// Must be called after Meter is set. Safe to skip if Meter is nil.
func (h *Handler) InitMetrics() {
	if h.Meter == nil {
		return
	}
	h.taskDuration, _ = h.Meter.Float64Histogram(
		"worker_task_duration_seconds",
		metric.WithDescription("Task processing duration in seconds"),
		metric.WithUnit("s"),
	)
	h.taskCount, _ = h.Meter.Int64Counter(
		"worker_tasks_processed_total",
		metric.WithDescription("Total tasks processed"),
	)
	h.activeTasks, _ = h.Meter.Int64UpDownCounter(
		"worker_active_tasks",
		metric.WithDescription("Number of in-flight tasks"),
	)
}
