package handlers

import (
	"github.com/batyray/notification-system/pkg/logger"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Handler struct {
	DB          *gorm.DB
	AsynqClient *asynq.Client
	Redis       *redis.Client
	Logger      *logger.Logger
}
