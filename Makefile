.PHONY: run stop test migrate-up migrate-down swagger lint

# Run all services
run:
	docker compose up --build -d

# Stop all services
stop:
	docker compose down

# Run tests
test:
	go test ./... -v -race -count=1

# Run tests with coverage
test-coverage:
	go test ./... -v -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

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
