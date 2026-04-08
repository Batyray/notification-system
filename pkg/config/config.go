package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Postgres     PostgresConfig
	Redis        RedisConfig
	API          APIConfig
	Worker       WorkerConfig
	Environment  string
	AppVersion   string
	OTLPEndpoint string
}

type PostgresConfig struct {
	Host     string
	Port     int
	DB       string
	User     string
	Password string
}

func (p PostgresConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
		p.Host, p.Port, p.DB, p.User, p.Password)
}

type RedisConfig struct {
	Addr string
}

type APIConfig struct {
	Port int
}

type WorkerConfig struct {
	WebhookURL         string
	RateLimitPerSecond int
	MetricsPort        int
}

func Load() (*Config, error) {
	// Load .env file if present (ignored if missing)
	_ = godotenv.Load()

	cfg := &Config{
		Postgres: PostgresConfig{
			Host:     envOrDefault("POSTGRES_HOST", "localhost"),
			Port:     envOrDefaultInt("POSTGRES_PORT", 5432),
			DB:       envOrDefault("POSTGRES_DB", "notifications"),
			User:     envOrDefault("POSTGRES_USER", "postgres"),
			Password: envOrDefault("POSTGRES_PASSWORD", "postgres"),
		},
		Redis: RedisConfig{
			Addr: envOrDefault("REDIS_ADDR", "localhost:6379"),
		},
		API: APIConfig{
			Port: envOrDefaultInt("API_PORT", 8080),
		},
		Worker: WorkerConfig{
			WebhookURL:         envOrDefault("WEBHOOK_URL", ""),
			RateLimitPerSecond: envOrDefaultInt("RATE_LIMIT_PER_SECOND", 100),
			MetricsPort:        envOrDefaultInt("METRICS_PORT", 9090),
		},
		Environment:  envOrDefault("ENVIRONMENT", "development"),
		AppVersion:   envOrDefault("APP_VERSION", "0.1.0"),
		OTLPEndpoint: envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318"),
	}

	return cfg, nil
}

func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func envOrDefaultInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	intVal, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return intVal
}
