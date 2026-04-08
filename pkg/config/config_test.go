package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	os.Clearenv()

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "localhost", cfg.Postgres.Host)
	assert.Equal(t, 5432, cfg.Postgres.Port)
	assert.Equal(t, "notifications", cfg.Postgres.DB)
	assert.Equal(t, "postgres", cfg.Postgres.User)
	assert.Equal(t, "postgres", cfg.Postgres.Password)
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, 8080, cfg.API.Port)
	assert.Equal(t, "development", cfg.Environment)
	assert.Equal(t, "0.1.0", cfg.AppVersion)
}

func TestLoad_FromEnv(t *testing.T) {
	os.Clearenv()
	t.Setenv("POSTGRES_HOST", "db.example.com")
	t.Setenv("POSTGRES_PORT", "5433")
	t.Setenv("REDIS_ADDR", "redis.example.com:6380")
	t.Setenv("API_PORT", "9090")
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("WEBHOOK_URL", "https://webhook.site/test-uuid")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "db.example.com", cfg.Postgres.Host)
	assert.Equal(t, 5433, cfg.Postgres.Port)
	assert.Equal(t, "redis.example.com:6380", cfg.Redis.Addr)
	assert.Equal(t, 9090, cfg.API.Port)
	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, "https://webhook.site/test-uuid", cfg.Worker.WebhookURL)
}

func TestPostgresConfig_DSN(t *testing.T) {
	cfg := PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		DB:       "notifications",
		User:     "postgres",
		Password: "postgres",
	}

	expected := "host=localhost port=5432 dbname=notifications user=postgres password=postgres sslmode=disable"
	assert.Equal(t, expected, cfg.DSN())
}
