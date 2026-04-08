package handlers

import (
	"github.com/batyray/notification-system/pkg/logger"
	"github.com/batyray/notification-system/services/worker/delivery"
	"gorm.io/gorm"
)

type Handler struct {
	DB             *gorm.DB
	DeliveryClient *delivery.Client
	Logger         *logger.Logger
}
