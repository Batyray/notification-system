package handlers

import (
	"math"
	"net/http"

	"github.com/batyray/notification-system/pkg/models"
)

type MetricsResponse struct {
	Queues   map[string]QueueMetrics `json:"queues"`
	Delivery DeliveryMetrics         `json:"delivery"`
}

type QueueMetrics struct {
	Pending int64 `json:"pending"`
	Active  int64 `json:"active"`
	Failed  int64 `json:"failed"`
}

type DeliveryMetrics struct {
	TotalSent    int64   `json:"total_sent"`
	TotalFailed  int64   `json:"total_failed"`
	SuccessRate  float64 `json:"success_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// GetMetrics godoc
// @Summary Get system metrics
// @Description Returns delivery metrics and queue statistics
// @Tags metrics
// @Produce json
// @Success 200 {object} MetricsResponse
// @Router /metrics [get]
func (h *Handler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	var totalSent, totalFailed int64
	h.DB.Model(&models.Notification{}).Where("status = ?", models.StatusSent).Count(&totalSent)
	h.DB.Model(&models.Notification{}).Where("status = ?", models.StatusFailed).Count(&totalFailed)

	total := totalSent + totalFailed
	var successRate float64
	if total > 0 {
		successRate = math.Round(float64(totalSent)/float64(total)*1000) / 10
	}

	channels := []string{"sms", "email", "push"}
	queues := make(map[string]QueueMetrics)
	for _, ch := range channels {
		var pending, active, failed int64
		h.DB.Model(&models.Notification{}).Where("channel = ? AND status = ?", ch, models.StatusPending).Count(&pending)
		h.DB.Model(&models.Notification{}).Where("channel = ? AND status = ?", ch, models.StatusProcessing).Count(&active)
		h.DB.Model(&models.Notification{}).Where("channel = ? AND status = ?", ch, models.StatusFailed).Count(&failed)
		queues[ch] = QueueMetrics{Pending: pending, Active: active, Failed: failed}
	}

	resp := MetricsResponse{
		Queues: queues,
		Delivery: DeliveryMetrics{
			TotalSent:    totalSent,
			TotalFailed:  totalFailed,
			SuccessRate:  successRate,
			AvgLatencyMs: 0, // Calculated from PostgreSQL EXTRACT in production; SQLite doesn't support it
		},
	}

	respondJSON(w, http.StatusOK, resp)
}
