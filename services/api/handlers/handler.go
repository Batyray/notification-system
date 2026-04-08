package handlers

import (
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/batyray/notification-system/pkg/logger"
	"gorm.io/gorm"
)

type Handler struct {
	DB          *gorm.DB
	AsynqClient *asynq.Client
	Redis       *redis.Client
	Logger      *logger.Logger
}
