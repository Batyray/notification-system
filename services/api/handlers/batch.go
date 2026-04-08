package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/batyray/notification-system/pkg/models"
	"github.com/batyray/notification-system/pkg/tasks"
	"github.com/batyray/notification-system/services/api/middleware"
)

type BatchCreateRequest struct {
	Notifications []CreateNotificationRequest `json:"notifications"`
}

type BatchCreateResponse struct {
	BatchID         uuid.UUID   `json:"batch_id"`
	NotificationIDs []uuid.UUID `json:"notification_ids"`
}

// CreateBatch godoc
// @Summary Create a batch of notifications
// @Description Create multiple notifications in a single request
// @Tags notifications
// @Accept json
// @Produce json
// @Param Idempotency-Key header string false "Idempotency key"
// @Param request body BatchCreateRequest true "Batch notification request"
// @Success 201 {object} BatchCreateResponse
// @Failure 400 {object} ErrorResponse
// @Router /notifications/batch [post]
func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	var req BatchCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	if len(req.Notifications) == 0 {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "notifications array is empty"})
		return
	}
	if len(req.Notifications) > 1000 {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "batch size exceeds limit of 1000"})
		return
	}

	// Validate all first
	for i, n := range req.Notifications {
		if n.Priority == "" {
			req.Notifications[i].Priority = string(models.PriorityNormal)
		}
		if err := validateCreateRequest(req.Notifications[i]); err != nil {
			respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
	}

	batchID := uuid.New()
	correlationID := middleware.GetCorrelationID(r.Context())
	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	notifications := make([]models.Notification, len(req.Notifications))
	for i, n := range req.Notifications {
		var templateVars *string
		if len(n.TemplateVars) > 0 {
			data, _ := json.Marshal(n.TemplateVars)
			s := string(data)
			templateVars = &s
		}

		notifications[i] = models.Notification{
			BatchID:       &batchID,
			Recipient:     n.Recipient,
			Channel:       models.Channel(n.Channel),
			Content:       n.Content,
			Priority:      models.Priority(n.Priority),
			Status:        models.StatusPending,
			CorrelationID: correlationID,
			TemplateVars:  templateVars,
			ScheduledAt:   n.ScheduledAt,
		}
	}

	if err := h.DB.Create(&notifications).Error; err != nil {
		respondJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to create batch"})
		return
	}

	var notificationIDs []uuid.UUID
	for _, notif := range notifications {
		notificationIDs = append(notificationIDs, notif.ID)
		if h.AsynqClient != nil {
			task, err := tasks.NewNotificationTask(tasks.NotificationPayload{
				NotificationID: notif.ID,
				Channel:        string(notif.Channel),
				Priority:       string(notif.Priority),
				CorrelationID:  correlationID,
			})
			if err == nil {
				opts := []asynq.Option{
					asynq.Queue(tasks.QueueName(string(notif.Channel), string(notif.Priority))),
					asynq.MaxRetry(5),
				}
				if notif.ScheduledAt != nil {
					opts = append(opts, asynq.ProcessAt(*notif.ScheduledAt))
				}
				h.AsynqClient.Enqueue(task, opts...)
			}
		}
	}

	respondJSON(w, http.StatusCreated, BatchCreateResponse{
		BatchID:         batchID,
		NotificationIDs: notificationIDs,
	})
}

// GetBatchNotifications godoc
// @Summary Get notifications by batch ID
// @Description Retrieve all notifications belonging to a batch
// @Tags notifications
// @Produce json
// @Param batchId path string true "Batch ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} ErrorResponse
// @Router /notifications/batch/{batchId} [get]
func (h *Handler) GetBatchNotifications(w http.ResponseWriter, r *http.Request) {
	batchIDStr := chi.URLParam(r, "batchId")
	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid batch ID"})
		return
	}

	var notifications []models.Notification
	if err := h.DB.Where("batch_id = ?", batchID).Order("created_at ASC").Find(&notifications).Error; err != nil {
		respondJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to fetch batch"})
		return
	}

	if len(notifications) == 0 {
		respondJSON(w, http.StatusNotFound, ErrorResponse{Error: "batch not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"batch_id": batchID,
		"count":    len(notifications),
		"data":     notifications,
	})
}
