package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/batyray/notification-system/pkg/models"
	"github.com/batyray/notification-system/pkg/tasks"
	"github.com/batyray/notification-system/services/api/middleware"
)

// CreateNotificationRequest represents the JSON body for creating a notification.
type CreateNotificationRequest struct {
	Recipient    string            `json:"recipient"`
	Channel      string            `json:"channel"`
	Content      string            `json:"content"`
	Priority     string            `json:"priority"`
	TemplateVars map[string]string `json:"template_vars,omitempty"`
	ScheduledAt  *time.Time        `json:"scheduled_at,omitempty"`
}

// ErrorResponse is used for returning error payloads.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// CreateNotification godoc
// @Summary Create a notification
// @Description Create a new notification request
// @Tags notifications
// @Accept json
// @Produce json
// @Param Idempotency-Key header string false "Idempotency key"
// @Param request body CreateNotificationRequest true "Notification request"
// @Success 201 {object} models.Notification
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /notifications [post]
func (h *Handler) CreateNotification(w http.ResponseWriter, r *http.Request) {
	var req CreateNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	// Validate required fields
	if req.Recipient == "" {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "recipient is required"})
		return
	}
	if req.Content == "" {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "content is required"})
		return
	}

	channel := models.Channel(req.Channel)
	if !channel.IsValid() {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("invalid channel: %s", req.Channel)})
		return
	}

	priority := models.Priority(req.Priority)
	if req.Priority == "" {
		priority = models.PriorityNormal
	}
	if !priority.IsValid() {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("invalid priority: %s", req.Priority)})
		return
	}

	// Correlation ID from middleware context, or generate a new one
	correlationID := middleware.GetCorrelationID(r.Context())
	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	// Template vars -> JSON string
	var templateVars *string
	if len(req.TemplateVars) > 0 {
		tvBytes, err := json.Marshal(req.TemplateVars)
		if err != nil {
			respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid template_vars"})
			return
		}
		s := string(tvBytes)
		templateVars = &s
	}

	notification := models.Notification{
		Recipient:     req.Recipient,
		Channel:       channel,
		Content:       req.Content,
		Priority:      priority,
		Status:        models.StatusPending,
		CorrelationID: correlationID,
		TemplateVars:  templateVars,
		ScheduledAt:   req.ScheduledAt,
	}

	if err := h.DB.Create(&notification).Error; err != nil {
		respondJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to create notification"})
		return
	}

	// Enqueue asynq task if client is available
	if h.AsynqClient != nil {
		payload := tasks.NotificationPayload{
			NotificationID: notification.ID,
			Channel:        string(notification.Channel),
			Priority:       string(notification.Priority),
			CorrelationID:  notification.CorrelationID,
		}

		task, err := tasks.NewNotificationTask(payload)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to create task"})
			return
		}

		opts := []asynq.Option{
			asynq.Queue(tasks.QueueName(string(notification.Channel), string(notification.Priority))),
			asynq.MaxRetry(5),
		}
		if notification.ScheduledAt != nil {
			opts = append(opts, asynq.ProcessAt(*notification.ScheduledAt))
		}

		if _, err := h.AsynqClient.Enqueue(task, opts...); err != nil {
			respondJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to enqueue task"})
			return
		}
	}

	respondJSON(w, http.StatusCreated, notification)
}

// GetNotification godoc
// @Summary Get a notification
// @Description Retrieve a single notification by its UUID
// @Tags notifications
// @Produce json
// @Param id path string true "Notification ID"
// @Success 200 {object} models.Notification
// @Failure 404 {object} ErrorResponse
// @Router /notifications/{id} [get]
func (h *Handler) GetNotification(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid notification id"})
		return
	}

	var notification models.Notification
	if err := h.DB.First(&notification, "id = ?", id).Error; err != nil {
		respondJSON(w, http.StatusNotFound, ErrorResponse{Error: "notification not found"})
		return
	}

	respondJSON(w, http.StatusOK, notification)
}

// CancelNotification godoc
// @Summary Cancel a notification
// @Description Cancel a notification only if it is in pending status
// @Tags notifications
// @Produce json
// @Param id path string true "Notification ID"
// @Success 200 {object} models.Notification
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /notifications/{id}/cancel [patch]
func (h *Handler) CancelNotification(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid notification id"})
		return
	}

	var notification models.Notification
	if err := h.DB.First(&notification, "id = ?", id).Error; err != nil {
		respondJSON(w, http.StatusNotFound, ErrorResponse{Error: "notification not found"})
		return
	}

	if notification.Status != models.StatusPending {
		respondJSON(w, http.StatusConflict, ErrorResponse{
			Error:   "cannot cancel notification",
			Details: fmt.Sprintf("notification is in '%s' status, only 'pending' notifications can be cancelled", notification.Status),
		})
		return
	}

	notification.Status = models.StatusCancelled
	if err := h.DB.Save(&notification).Error; err != nil {
		respondJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to cancel notification"})
		return
	}

	respondJSON(w, http.StatusOK, notification)
}

// ListNotifications godoc
// @Summary List notifications
// @Description Returns a cursor-paginated list of notifications with optional filters
// @Tags notifications
// @Produce json
// @Param status query string false "Filter by status"
// @Param channel query string false "Filter by channel"
// @Param from query string false "Filter by created_at >= from (RFC3339)"
// @Param to query string false "Filter by created_at <= to (RFC3339)"
// @Param cursor query string false "Pagination cursor"
// @Param page_size query string false "Number of results per page"
// @Success 200 {object} map[string]interface{}
// @Router /notifications [get]
func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	pageSize := 20
	if ps := q.Get("page_size"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n > 0 {
			pageSize = n
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := h.DB.Model(&models.Notification{})

	// Apply filters
	if status := q.Get("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if channel := q.Get("channel"); channel != "" {
		query = query.Where("channel = ?", channel)
	}
	if from := q.Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if to := q.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			query = query.Where("created_at <= ?", t)
		}
	}

	// Apply cursor
	if cursor := q.Get("cursor"); cursor != "" {
		cursorTime, cursorID, err := decodeCursor(cursor)
		if err == nil {
			query = query.Where(
				"(created_at < ?) OR (created_at = ? AND id < ?)",
				cursorTime, cursorTime, cursorID,
			)
		}
	}

	// Order by created_at desc, id desc for stable sort
	query = query.Order("created_at DESC, id DESC")

	// Fetch pageSize+1 to determine if there's a next page
	var notifications []models.Notification
	if err := query.Limit(pageSize + 1).Find(&notifications).Error; err != nil {
		respondJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to list notifications"})
		return
	}

	var nextCursor *string
	if len(notifications) > pageSize {
		// There are more results; build cursor from the last item we'll return
		last := notifications[pageSize-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		nextCursor = &c
		notifications = notifications[:pageSize]
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"data":        notifications,
		"next_cursor": nextCursor,
	})
}

// --- Cursor helpers ---

func encodeCursor(t time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%d:%s", t.UnixNano(), id.String())
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}

	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor format")
	}

	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}

	return time.Unix(0, nanos), id, nil
}

// validateCreateRequest validates a CreateNotificationRequest and returns an error if invalid.
func validateCreateRequest(req CreateNotificationRequest) error {
	if req.Recipient == "" {
		return fmt.Errorf("recipient is required")
	}
	if req.Content == "" {
		return fmt.Errorf("content is required")
	}
	channel := models.Channel(req.Channel)
	if !channel.IsValid() {
		return fmt.Errorf("invalid channel: %s", req.Channel)
	}
	priority := models.Priority(req.Priority)
	if !priority.IsValid() {
		return fmt.Errorf("invalid priority: %s", req.Priority)
	}
	return nil
}

// --- Response helper ---

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}
