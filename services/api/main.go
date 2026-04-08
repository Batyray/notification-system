package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	_ "github.com/batyray/notification-system/docs/swagger"
	"github.com/batyray/notification-system/pkg/config"
	"github.com/batyray/notification-system/pkg/logger"
	"github.com/batyray/notification-system/pkg/metrics"
	"github.com/batyray/notification-system/pkg/tracing"
	"github.com/batyray/notification-system/services/api/handlers"
	"github.com/batyray/notification-system/services/api/router"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// @title Notification System API
// @version 1.0
// @description Event-driven notification system for SMS, Email, and Push channels
// @host localhost:8080
// @BasePath /api/v1

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

	shutdownTracer, err := tracing.Init(context.Background(), "api", cfg.OTLPEndpoint)
	if err != nil {
		l.Warn("failed to init tracing, continuing without it", "error", err)
	} else {
		defer func() { _ = shutdownTracer(context.Background()) }()
		l.Info("tracing initialized")
	}

	metricsHandler, shutdownMetrics, err := metrics.Init("api")
	if err != nil {
		l.Warn("failed to init metrics, continuing without it", "error", err)
	} else {
		defer func() { _ = shutdownMetrics(context.Background()) }()
		l.Info("metrics initialized")
	}

	db, err := gorm.Open(postgres.Open(cfg.Postgres.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	l.Info("connected to postgresql")

	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		l.Warn("failed to add GORM tracing plugin", "error", err)
	}

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
		Handler:        h,
		Redis:          rdb,
		Logger:         l,
		MetricsHandler: metricsHandler,
	})

	addr := fmt.Sprintf(":%d", cfg.API.Port)
	l.Info("api server starting", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
