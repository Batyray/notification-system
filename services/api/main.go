package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/batyray/notification-system/pkg/config"
	"github.com/batyray/notification-system/pkg/logger"
	"github.com/batyray/notification-system/services/api/handlers"
	"github.com/batyray/notification-system/services/api/router"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	l := logger.New(logger.Options{
		Service:     "api",
		Environment: cfg.Environment,
		Version:     cfg.AppVersion,
	})

	db, err := gorm.Open(postgres.Open(cfg.Postgres.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	l.Info("connected to postgresql")

	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Addr,
	})
	l.Info("connected to redis")

	asynqClient := asynq.NewClient(asynq.RedisClientOpt{
		Addr: cfg.Redis.Addr,
	})
	defer asynqClient.Close()

	h := &handlers.Handler{
		DB:          db,
		AsynqClient: asynqClient,
		Redis:       rdb,
		Logger:      l,
	}

	r := router.New(router.Deps{
		Handler: h,
		Redis:   rdb,
		Logger:  l,
	})

	addr := fmt.Sprintf(":%d", cfg.API.Port)
	l.Info("api server starting", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
