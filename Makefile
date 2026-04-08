.PHONY: run stop test test-integration migrate-up migrate-down swagger lint

# Run all services
run:
	docker compose up --build -d

# Stop all services
stop:
	docker compose down

# Run tests
test:
	GOTOOLCHAIN=auto go test ./... -v -race -count=1

# Run tests with coverage
test-coverage:
	GOTOOLCHAIN=auto go test ./... -v -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Run integration tests (requires docker-compose up)
test-integration:
	GOTOOLCHAIN=auto go test -tags=integration ./tests/integration/... -v -count=1

# Run migrations (local dev — assumes postgres on localhost)
migrate-up:
	migrate -path migrations -database "postgres://postgres:postgres@localhost:5432/notifications?sslmode=disable" up

migrate-down:
	migrate -path migrations -database "postgres://postgres:postgres@localhost:5432/notifications?sslmode=disable" down 1

# Generate swagger docs
swagger:
	swag init -g services/api/main.go -o docs/swagger --parseDependency

# Lint
lint:
	golangci-lint run ./...
