package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/batyray/notification-system/pkg/config"
	"github.com/batyray/notification-system/pkg/logger"
	"github.com/batyray/notification-system/pkg/models"
	"github.com/batyray/notification-system/pkg/tasks"
	"github.com/batyray/notification-system/pkg/tracing"
	"github.com/batyray/notification-system/services/worker/delivery"
	"github.com/batyray/notification-system/services/worker/handlers"
	"github.com/batyray/notification-system/services/worker/ratelimit"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var retryDelays = []time.Duration{
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
}

func retryDelayFunc(n int, err error, _ *asynq.Task) time.Duration {
	var rateLimitErr *ratelimit.RateLimitedError
	if errors.As(err, &rateLimitErr) {
		return 100 * time.Millisecond
	}
	if n < len(retryDelays) {
		return retryDelays[n]
	}
	return retryDelays[len(retryDelays)-1]
}

type errorHandler struct {
	db     *gorm.DB
	logger *logger.Logger
}

func (eh *errorHandler) HandleError(ctx context.Context, task *asynq.Task, err error) {
	payload, parseErr := tasks.ParseNotificationPayload(task)
	if parseErr != nil {
		eh.logger.Error("failed to parse payload in error handler", "error", parseErr)
		return
	}

	retries, _ := asynq.GetRetryCount(ctx)
	maxRetries, _ := asynq.GetMaxRetry(ctx)

	if retries >= maxRetries {
		errMsg := fmt.Sprintf("permanently failed after %d retries: %v", retries, err)
		eh.db.Model(&models.Notification{}).
			Where("id = ?", payload.NotificationID).
			Updates(map[string]interface{}{
				"status":        models.StatusFailed,
				"error_message": errMsg,
			})
		eh.logger.Error("notification permanently failed",
			"notification_id", payload.NotificationID,
			"retries", retries,
			"error", err,
		)
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	l := logger.New(logger.Options{
		Service:     "worker",
		Environment: cfg.Environment,
		Version:     cfg.AppVersion,
	})

	shutdownTracer, err := tracing.Init(context.Background(), "worker", cfg.OTLPEndpoint)
	if err != nil {
		l.Warn("failed to init tracing, continuing without it", "error", err)
	} else {
		defer shutdownTracer(context.Background())
		l.Info("tracing initialized")
	}

	db, err := gorm.Open(postgres.Open(cfg.Postgres.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	l.Info("connected to postgresql")

	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		l.Warn("failed to add GORM tracing plugin", "error", err)
	}

	deliveryClient := delivery.NewClient(cfg.Worker.WebhookURL)

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	l.Info("connected to redis for rate limiting")

	limiter := ratelimit.New(rdb, cfg.Worker.RateLimitPerSecond, 1*time.Second)

	h := &handlers.Handler{
		DB:             db,
		DeliveryClient: deliveryClient,
		Logger:         l,
		Limiter:        limiter,
	}

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.Redis.Addr},
		asynq.Config{
			Concurrency:    50,
			Queues:         tasks.AllQueues(),
			RetryDelayFunc: retryDelayFunc,
			ErrorHandler: &errorHandler{
				db:     db,
				logger: l,
			},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeNotificationSMS, h.HandleNotification)
	mux.HandleFunc(tasks.TypeNotificationEmail, h.HandleNotification)
	mux.HandleFunc(tasks.TypeNotificationPush, h.HandleNotification)

	l.Info("worker starting",
		"concurrency", 50,
		"queues", len(tasks.AllQueues()),
		"rate_limit_per_second", cfg.Worker.RateLimitPerSecond,
	)
	if err := srv.Run(mux); err != nil {
		log.Fatalf("worker failed: %v", err)
	}
}
