package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"github.com/batyray/notification-system/pkg/models"
	"github.com/batyray/notification-system/pkg/tasks"
	"github.com/batyray/notification-system/services/worker/delivery"
)

func (h *Handler) HandleNotification(ctx context.Context, task *asynq.Task) error {
	payload, err := tasks.ParseNotificationPayload(task)
	if err != nil {
		h.Logger.Error("failed to parse payload", "error", err)
		return fmt.Errorf("parse payload: %w", err)
	}

	// Extract trace context from the task payload (propagated from API).
	if payload.TraceCarrier != nil {
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(payload.TraceCarrier))
	}

	tracer := otel.Tracer("worker")
	ctx, span := tracer.Start(ctx, "worker.HandleNotification",
		trace.WithAttributes(
			attribute.String("notification.id", payload.NotificationID.String()),
			attribute.String("notification.channel", payload.Channel),
			attribute.String("notification.correlation_id", payload.CorrelationID),
		),
	)
	defer span.End()

	h.Logger.Info("processing notification",
		"notification_id", payload.NotificationID,
		"channel", payload.Channel,
		"correlation_id", payload.CorrelationID,
	)

	// Wait for rate limit before processing. Retries inline to avoid
	// consuming Asynq's MaxRetry budget on transient throttling.
	if h.Limiter != nil {
		for {
			allowed, err := h.Limiter.Allow(ctx, payload.Channel)
			if err != nil {
				h.Logger.Error("rate limit check failed", "error", err, "channel", payload.Channel)
				return fmt.Errorf("rate limit check: %w", err)
			}
			if allowed {
				break
			}
			h.Logger.Debug("rate limited, waiting",
				"channel", payload.Channel,
				"notification_id", payload.NotificationID,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	var notification models.Notification
	if err := h.DB.First(&notification, "id = ?", payload.NotificationID).Error; err != nil {
		h.Logger.Error("failed to fetch notification", "error", err, "notification_id", payload.NotificationID)
		return fmt.Errorf("fetch notification: %w", err)
	}

	if notification.Status == models.StatusCancelled {
		h.Logger.Info("notification cancelled, skipping", "notification_id", payload.NotificationID)
		return nil
	}

	notification.Status = models.StatusProcessing
	notification.RetryCount++
	if err := h.DB.Save(&notification).Error; err != nil {
		h.Logger.Error("failed to update status to processing", "error", err)
		return fmt.Errorf("update status: %w", err)
	}

	content := notification.Content
	if notification.TemplateVars != nil && *notification.TemplateVars != "" {
		rendered, err := renderTemplate(content, *notification.TemplateVars)
		if err != nil {
			h.Logger.Error("failed to render template", "error", err)
			errMsg := err.Error()
			notification.Status = models.StatusFailed
			notification.ErrorMessage = &errMsg
			h.DB.Save(&notification)
			return asynq.SkipRetry
		}
		content = rendered
	}

	deliveryCtx, deliverySpan := tracer.Start(ctx, "webhook.deliver",
		trace.WithAttributes(
			attribute.String("notification.id", payload.NotificationID.String()),
			attribute.String("notification.channel", string(notification.Channel)),
			attribute.String("notification.recipient", notification.Recipient),
		),
	)
	defer deliverySpan.End()

	resp, err := h.DeliveryClient.Send(deliveryCtx, delivery.SendRequest{
		To:            notification.Recipient,
		Channel:       string(notification.Channel),
		Content:       content,
		CorrelationID: notification.CorrelationID,
	})

	if err != nil {
		deliverySpan.RecordError(err)
		deliverySpan.SetStatus(codes.Error, err.Error())
		var sendErr *delivery.SendError
		if errors.As(err, &sendErr) {
			deliverySpan.SetAttributes(attribute.Int("http.status_code", sendErr.StatusCode))
		}
	} else {
		// Client.Send only succeeds on 202 Accepted
		deliverySpan.SetAttributes(attribute.Int("http.status_code", 202))
		deliverySpan.SetStatus(codes.Ok, "")
	}

	if err != nil {
		var sendErr *delivery.SendError
		if errors.As(err, &sendErr) {
			errMsg := sendErr.Error()
			notification.ErrorMessage = &errMsg

			if sendErr.IsTransient() {
				notification.Status = models.StatusFailed
				h.DB.Save(&notification)
				h.Logger.Warn("transient delivery error, will retry",
					"notification_id", payload.NotificationID,
					"status_code", sendErr.StatusCode,
				)
				return fmt.Errorf("transient delivery error: %w", err)
			}

			notification.Status = models.StatusFailed
			h.DB.Save(&notification)
			h.Logger.Error("permanent delivery error",
				"notification_id", payload.NotificationID,
				"status_code", sendErr.StatusCode,
			)
			return asynq.SkipRetry
		}

		errMsg := err.Error()
		notification.Status = models.StatusFailed
		notification.ErrorMessage = &errMsg
		h.DB.Save(&notification)
		return fmt.Errorf("delivery error: %w", err)
	}

	now := time.Now()
	notification.Status = models.StatusSent
	notification.ProviderMessageID = &resp.MessageID
	notification.SentAt = &now
	if err := h.DB.Save(&notification).Error; err != nil {
		h.Logger.Error("failed to update notification as sent", "error", err)
		return fmt.Errorf("update sent status: %w", err)
	}

	h.Logger.Info("notification sent successfully",
		"notification_id", payload.NotificationID,
		"provider_message_id", resp.MessageID,
	)

	return nil
}

func renderTemplate(content string, templateVarsJSON string) (string, error) {
	var vars map[string]string
	if err := json.Unmarshal([]byte(templateVarsJSON), &vars); err != nil {
		return "", fmt.Errorf("unmarshal template vars: %w", err)
	}

	tmpl, err := template.New("notification").Parse(content)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
