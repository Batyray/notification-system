package tasks

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const (
	TypeNotificationSMS   = "notification:sms"
	TypeNotificationEmail = "notification:email"
	TypeNotificationPush  = "notification:push"
)

type NotificationPayload struct {
	NotificationID uuid.UUID         `json:"notification_id"`
	Channel        string            `json:"channel"`
	Priority       string            `json:"priority"`
	CorrelationID  string            `json:"correlation_id"`
	TraceCarrier   map[string]string `json:"trace_carrier,omitempty"`
}

func NewNotificationTask(payload NotificationPayload) (*asynq.Task, error) {
	taskType, err := taskTypeForChannel(payload.Channel)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	return asynq.NewTask(taskType, data), nil
}

func ParseNotificationPayload(task *asynq.Task) (*NotificationPayload, error) {
	var payload NotificationPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	return &payload, nil
}

func QueueName(channel, priority string) string {
	return channel + ":" + priority
}

func AllQueues() map[string]int {
	return map[string]int{
		"sms:high": 6, "email:high": 6, "push:high": 6,
		"sms:normal": 3, "email:normal": 3, "push:normal": 3,
		"sms:low": 1, "email:low": 1, "push:low": 1,
	}
}

func taskTypeForChannel(channel string) (string, error) {
	switch channel {
	case "sms":
		return TypeNotificationSMS, nil
	case "email":
		return TypeNotificationEmail, nil
	case "push":
		return TypeNotificationPush, nil
	default:
		return "", fmt.Errorf("unsupported channel: %s", channel)
	}
}
