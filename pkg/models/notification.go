package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Channel string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"
)

func (c Channel) IsValid() bool {
	switch c {
	case ChannelSMS, ChannelEmail, ChannelPush:
		return true
	}
	return false
}

type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"
)

func (p Priority) IsValid() bool {
	switch p {
	case PriorityHigh, PriorityNormal, PriorityLow:
		return true
	}
	return false
}

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusSent       Status = "sent"
	StatusFailed     Status = "failed"
	StatusCancelled  Status = "cancelled"
)

type Notification struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	BatchID           *uuid.UUID `gorm:"type:uuid;index" json:"batch_id,omitempty"`
	IdempotencyKey    *string    `gorm:"type:varchar(255);uniqueIndex" json:"idempotency_key,omitempty"`
	Recipient         string     `gorm:"type:varchar(255);not null" json:"recipient"`
	Channel           Channel    `gorm:"type:varchar(10);not null" json:"channel"`
	Content           string     `gorm:"type:text;not null" json:"content"`
	Priority          Priority   `gorm:"type:varchar(10);not null;default:'normal'" json:"priority"`
	Status            Status     `gorm:"type:varchar(20);not null;default:'pending';index" json:"status"`
	ProviderMessageID *string    `gorm:"type:varchar(255)" json:"provider_message_id,omitempty"`
	RetryCount        int        `gorm:"default:0" json:"retry_count"`
	ErrorMessage      *string    `gorm:"type:text" json:"error_message,omitempty"`
	CorrelationID     string     `gorm:"type:varchar(255)" json:"correlation_id"`
	TemplateVars      *string    `gorm:"type:text" json:"template_vars,omitempty"`
	ScheduledAt       *time.Time `gorm:"index" json:"scheduled_at,omitempty"`
	CreatedAt         time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	SentAt            *time.Time `json:"sent_at,omitempty"`
}

func (n *Notification) BeforeCreate(tx *gorm.DB) error {
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	return nil
}

// QueueName returns the asynq queue name for this notification (e.g., "sms:high")
func (n *Notification) QueueName() string {
	return string(n.Channel) + ":" + string(n.Priority)
}
