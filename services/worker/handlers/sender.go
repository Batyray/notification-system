package handlers

import (
	"context"

	"github.com/batyray/notification-system/services/worker/delivery"
)

// Sender abstracts notification delivery so tests can substitute a mock.
type Sender interface {
	Send(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error)
}
