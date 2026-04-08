.PHONY: run stop restart logs clean test test-coverage test-integration lint fmt swagger setup deps

GOBIN := $(shell go env GOPATH)/bin
GOLANGCI_LINT := $(GOBIN)/golangci-lint
SWAG := $(GOBIN)/swag
MIGRATE := $(shell command -v migrate 2>/dev/null || echo $(GOBIN)/migrate)

# ─── Docker Compose ──────────────────────────────────────────

## Start all services
run:
	docker compose up --build -d

## Stop all services
stop:
	docker compose down

## Restart all services
restart:
	docker compose down && docker compose up --build -d

## Tail logs (all services, follow mode)
logs:
	docker compose logs -f

## Stop services and remove volumes (full reset)
clean:
	docker compose down -v

# ─── Testing ─────────────────────────────────────────────────

## Run unit tests with race detection
test:
	GOTOOLCHAIN=auto go test ./... -v -race -count=1

## Run unit tests with coverage report
test-coverage:
	GOTOOLCHAIN=auto go test ./... -v -race -coverprofile=coverage.out
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## Run integration tests (requires 'make run' first)
test-integration:
	GOTOOLCHAIN=auto go test -tags=integration ./tests/integration/... -v -count=1

## Run test scenarios script against running services
test-scenarios:
	bash scripts/test_scenarios.sh

# ─── Code Quality ────────────────────────────────────────────

## Run linter
lint:
	$(GOLANGCI_LINT) run ./...

## Format code
fmt:
	gofmt -w .

## Check formatting (fails if unformatted — useful in CI)
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Unformatted files:" && gofmt -l . && exit 1)

# ─── Code Generation ────────────────────────────────────────

## Generate swagger docs
swagger:
	$(SWAG) init -g services/api/main.go -o docs/swagger --parseDependency

## Check swagger docs are up to date (useful in CI)
swagger-check: swagger
	@git diff --quiet docs/swagger/ || (echo "Swagger docs are out of date. Run 'make swagger' and commit." && exit 1)

# ─── Database ────────────────────────────────────────────────

## Run migrations up (local dev)
migrate-up:
	$(MIGRATE) -path migrations -database "postgres://postgres:postgres@localhost:5432/notifications?sslmode=disable" up

## Roll back one migration (local dev)
migrate-down:
	$(MIGRATE) -path migrations -database "postgres://postgres:postgres@localhost:5432/notifications?sslmode=disable" down 1

# ─── Setup ───────────────────────────────────────────────────

## Install dev dependencies (skips already-installed tools)
deps:
	@echo "Installing dev dependencies..."
	@test -x $(GOLANGCI_LINT) && echo "golangci-lint: already installed" || \
		(echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@test -x $(SWAG) && echo "swag: already installed" || \
		(echo "Installing swag..." && go install github.com/swaggo/swag/cmd/swag@latest)
	@command -v migrate >/dev/null 2>&1 || test -x $(GOBIN)/migrate && echo "migrate: already installed" || \
		( \
		echo "Installing golang-migrate..." && \
		OS=$$(uname -s | tr '[:upper:]' '[:lower:]') && \
		ARCH=$$(uname -m) && \
		if [ "$$ARCH" = "x86_64" ]; then ARCH="amd64"; elif [ "$$ARCH" = "aarch64" ] || [ "$$ARCH" = "arm64" ]; then ARCH="arm64"; fi && \
		if [ "$$OS" = "darwin" ]; then \
			if command -v brew >/dev/null 2>&1; then \
				brew install golang-migrate; \
			else \
				curl -L "https://github.com/golang-migrate/migrate/releases/latest/download/migrate.$$OS-$$ARCH.tar.gz" | tar xz -C $$(go env GOPATH)/bin; \
			fi; \
		elif [ "$$OS" = "linux" ]; then \
			curl -L "https://github.com/golang-migrate/migrate/releases/latest/download/migrate.$$OS-$$ARCH.tar.gz" | tar xz -C $$(go env GOPATH)/bin; \
		else \
			echo "Unsupported OS: $$OS — install golang-migrate manually: https://github.com/golang-migrate/migrate"; \
		fi \
		)
	@echo "Done. All dev tools installed."

## Copy .env.example to .env if it doesn't exist
setup: deps
	@test -f .env || (cp .env.example .env && echo "Created .env from .env.example — edit it with your values") || true
	@test -f .env && echo ".env already exists" || true

# ─── Help ────────────────────────────────────────────────────

## Show available commands
help:
	@echo "Available targets:"
	@echo ""
	@echo "  Docker:"
	@echo "    make run              Start all services"
	@echo "    make stop             Stop all services"
	@echo "    make restart          Restart all services"
	@echo "    make logs             Tail service logs"
	@echo "    make clean            Stop and remove volumes"
	@echo ""
	@echo "  Testing:"
	@echo "    make test             Run unit tests"
	@echo "    make test-coverage    Run tests with coverage"
	@echo "    make test-integration Run integration tests"
	@echo "    make test-scenarios   Run API test scenarios"
	@echo ""
	@echo "  Code Quality:"
	@echo "    make lint             Run linter"
	@echo "    make fmt              Format code"
	@echo "    make fmt-check        Check formatting (CI)"
	@echo ""
	@echo "  Code Generation:"
	@echo "    make swagger          Generate swagger docs"
	@echo "    make swagger-check    Verify swagger is up to date (CI)"
	@echo ""
	@echo "  Database:"
	@echo "    make migrate-up       Run migrations"
	@echo "    make migrate-down     Roll back one migration"
	@echo ""
	@echo "  Setup:"
	@echo "    make deps             Install dev tools (lint, swag, migrate)"
	@echo "    make setup            Install deps + create .env from example"
