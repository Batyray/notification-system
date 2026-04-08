package handlers

import (
	"context"

	"github.com/batyray/notification-system/services/worker/delivery"
)

type mockSender struct {
	SendFunc func(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error)
	Calls    []delivery.SendRequest
}

func (m *mockSender) Send(ctx context.Context, req delivery.SendRequest) (*delivery.SendResponse, error) {
	m.Calls = append(m.Calls, req)
	return m.SendFunc(ctx, req)
}
