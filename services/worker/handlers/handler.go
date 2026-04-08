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
}
