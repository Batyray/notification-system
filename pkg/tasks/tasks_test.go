package tasks

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNotificationTask(t *testing.T) {
	id := uuid.New()
	task, err := NewNotificationTask(NotificationPayload{
		NotificationID: id,
		Channel:        "sms",
		Priority:       "high",
		CorrelationID:  "corr-123",
	})
	require.NoError(t, err)
	assert.Equal(t, TypeNotificationSMS, task.Type())
}

func TestNewNotificationTask_AllChannels(t *testing.T) {
	tests := []struct {
		channel  string
		taskType string
	}{
		{"sms", TypeNotificationSMS},
		{"email", TypeNotificationEmail},
		{"push", TypeNotificationPush},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			task, err := NewNotificationTask(NotificationPayload{
				NotificationID: uuid.New(),
				Channel:        tt.channel,
				Priority:       "normal",
				CorrelationID:  "corr-456",
			})
			require.NoError(t, err)
			assert.Equal(t, tt.taskType, task.Type())
		})
	}
}

func TestNewNotificationTask_InvalidChannel(t *testing.T) {
	_, err := NewNotificationTask(NotificationPayload{
		NotificationID: uuid.New(),
		Channel:        "telegram",
		Priority:       "normal",
		CorrelationID:  "corr-789",
	})
	assert.Error(t, err)
}

func TestParseNotificationPayload(t *testing.T) {
	id := uuid.New()
	original := NotificationPayload{
		NotificationID: id,
		Channel:        "email",
		Priority:       "low",
		CorrelationID:  "corr-abc",
	}

	task, err := NewNotificationTask(original)
	require.NoError(t, err)

	parsed, err := ParseNotificationPayload(task)
	require.NoError(t, err)

	assert.Equal(t, original.NotificationID, parsed.NotificationID)
	assert.Equal(t, original.Channel, parsed.Channel)
	assert.Equal(t, original.Priority, parsed.Priority)
	assert.Equal(t, original.CorrelationID, parsed.CorrelationID)
}

func TestQueueName(t *testing.T) {
	assert.Equal(t, "sms:high", QueueName("sms", "high"))
	assert.Equal(t, "email:normal", QueueName("email", "normal"))
	assert.Equal(t, "push:low", QueueName("push", "low"))
}

func TestParseNotificationPayload_WithTraceCarrier(t *testing.T) {
	id := uuid.New()
	original := NotificationPayload{
		NotificationID: id,
		Channel:        "sms",
		Priority:       "normal",
		CorrelationID:  "corr-trace",
		TraceCarrier: map[string]string{
			"traceparent": "00-4bf92f3577b6a814af67d78dd14f1a00-00f067aa0ba902b7-01",
		},
	}

	task, err := NewNotificationTask(original)
	require.NoError(t, err)

	parsed, err := ParseNotificationPayload(task)
	require.NoError(t, err)

	assert.Equal(t, original.TraceCarrier, parsed.TraceCarrier)
	assert.Equal(t, "00-4bf92f3577b6a814af67d78dd14f1a00-00f067aa0ba902b7-01", parsed.TraceCarrier["traceparent"])
}

func TestParseNotificationPayload_NilTraceCarrier(t *testing.T) {
	id := uuid.New()
	original := NotificationPayload{
		NotificationID: id,
		Channel:        "email",
		Priority:       "high",
		CorrelationID:  "corr-nil",
	}

	task, err := NewNotificationTask(original)
	require.NoError(t, err)

	parsed, err := ParseNotificationPayload(task)
	require.NoError(t, err)

	assert.Nil(t, parsed.TraceCarrier)
}
