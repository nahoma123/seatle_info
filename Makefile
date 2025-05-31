# File: Makefile

.PHONY: all build run test coverage clean tidy help wire migrate-up migrate-down migrate-create docker-dev-up docker-dev-down docker-dev-logs

# Variables
APP_NAME=seattle_info_backend
CMD_DIR=./cmd/server
BUILD_DIR=./build
DOCKER_COMPOSE_DEV=docker compose -f docker-compose.dev.yml
MIGRATE_PATH=./migrations
MIGRATE_DSN_ENV_VAR=DB_SOURCE # Assumes DB_SOURCE is set in your .env for migrate tool

# Go commands
GO=go
GO_BUILD=$(GO) build
GO_RUN=$(GO) run
GO_TEST=$(GO) test
GO_CLEAN=$(GO) clean
GO_MOD_TIDY=$(GO) mod tidy
GO_GET=$(GO) get

# Wire command
WIRE_CMD=wire

all: build

# Build the application binary
build: tidy
	@echo "Building $(APP_NAME)..."
	$(GO_BUILD) -o $(BUILD_DIR)/$(APP_NAME) $(CMD_DIR)/main.go $(CMD_DIR)/wire_gen.go

# Run the application
run:
	@echo "Running $(APP_NAME)..."
	$(GO_RUN) $(CMD_DIR)/main.go $(CMD_DIR)/wire_gen.go

# Run tests
test:
	@echo "Running tests..."
	$(GO_TEST) -v ./...

# Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	$(GO_TEST) -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GO_CLEAN)
	rm -f $(BUILD_DIR)/$(APP_NAME)
	rm -f coverage.out coverage.html

# Tidy go.mod and go.sum
tidy:
	@echo "Tidying dependencies..."
	$(GO_MOD_TIDY)

# Generate Wire code
wire:
	@echo "Generating Wire code..."
	cd $(CMD_DIR) && $(WIRE_CMD)
	@echo "Wire code generation complete."

# Database Migrations (using golang-migrate/migrate)
# Ensure you have migrate CLI installed: https://github.com/golang-migrate/migrate/tree/master/cmd/migrate
# And DB_SOURCE environment variable is set, e.g., in your .env file for local development
# export DB_SOURCE="postgresql://user:password@localhost:5432/dbname?sslmode=disable"

migrate-up:
    @echo "Applying migrations..."
	docker compose -f docker-compose.dev.yml run --rm migrate 'migrate -path /migrations -database "$DB_SOURCE" up'
	
migrate-down:
	@echo "Reverting last migration..."
	@if [ -z "$$DB_SOURCE" ]; then echo "Error: DB_SOURCE environment variable is not set."; exit 1; fi
	docker compose -f docker-compose.dev.yml run --rm -e DB_SOURCE migrate migrate -database "$$DB_SOURCE" -path /migrations down 1

# Docker development environment
docker-dev-up:
	@echo "Starting development environment with Docker Compose..."
	$(DOCKER_COMPOSE_DEV) up -d --build

docker-dev-down:
	@echo "Stopping development environment..."
	$(DOCKER_COMPOSE_DEV) down

docker-dev-logs:
	@echo "Showing logs for development environment..."
	$(DOCKER_COMPOSE_DEV) logs -f

# Help
help:
	@echo "Available commands:"
	@echo "  build          - Build the application"
	@echo "  run            - Run the application"
	@echo "  test           - Run tests"
	@echo "  coverage       - Run tests with coverage"
	@echo "  clean          - Clean build artifacts"
	@echo "  tidy           - Tidy go.mod and go.sum"
	@echo "  wire           - Generate Wire dependency injection code"
	@echo "  migrate-up     - Apply all database migrations"
	@echo "  migrate-down   - Revert the last database migration"
	@echo "  migrate-create NAME=<migration_name> - Create new migration files"
	@echo "  docker-dev-up  - Start development environment with Docker Compose"
	@echo "  docker-dev-down- Stop development environment"
	@echo "  docker-dev-logs- Show logs for development environment"