# File: Makefile

.PHONY: all build run test coverage clean tidy help wire migrate-up migrate-down migrate-create docker-dev-up docker-dev-down docker-dev-logs docker-prod-up docker-prod-down docker-prod-logs

# Variables
APP_NAME=seattle_info_backend
CMD_DIR=./cmd/server
BUILD_DIR=./build
DOCKER_COMPOSE_DEV=docker compose -f docker-compose.dev.yml
DOCKER_COMPOSE_PROD=docker compose -f docker-compose.yml
MIGRATE_PATH=./migrations
MIGRATE_DSN_ENV_VAR=DB_SOURCE # Assumes DB_SOURCE is set in your .env.dev for migrate tool

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
# And DB_SOURCE environment variable is set, e.g., in your .env.dev file for local development
# To ensure make has access to .env.dev variables for migration commands, you can:
# 1. Source it manually: `source .env.dev && make migrate-up`
# 2. Use a tool like `direnv` which automatically loads .env files.
# 3. Prepend the variable assignment: `DB_SOURCE=$(grep DB_SOURCE .env.dev | cut -d '=' -f2-) make migrate-up` (complex)
# For simplicity, we assume DB_SOURCE from .env.dev is available in the environment when running make migrate commands.
# The docker-compose.dev.yml for the migrate service itself also loads .env.dev.

migrate-up:
	@echo "Applying migrations (using DB_SOURCE from .env.dev via docker-compose.dev.yml)..."
	# The migrate service in docker-compose.dev.yml is configured to use .env.dev
	# So, DB_SOURCE should be available within the container when the command runs.
	$(DOCKER_COMPOSE_DEV) run --rm migrate migrate -path /migrations -database "$${DB_SOURCE}" up

migrate-down:
	@echo "Reverting last migration (using DB_SOURCE from .env.dev via docker-compose.dev.yml)..."
	# Similar to migrate-up, relying on the migrate service's env_file config
	$(DOCKER_COMPOSE_DEV) run --rm migrate migrate -path /migrations -database "$${DB_SOURCE}" down 1

migrate-create:
	@if [ -z "$$NAME" ]; then echo "Error: NAME environment variable is not set. Usage: make migrate-create NAME=<migration_name>"; exit 1; fi
	@echo "Creating new migration files for: $$NAME..."
	$(DOCKER_COMPOSE_DEV) run --rm migrate migrate create -ext sql -dir /migrations -seq "$$NAME"

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

# Docker production environment
docker-prod-up:
	@echo "Starting production environment with Docker Compose..."
	$(DOCKER_COMPOSE_PROD) up -d --build

docker-prod-down:
	@echo "Stopping production environment..."
	$(DOCKER_COMPOSE_PROD) down

docker-prod-logs:
	@echo "Showing logs for production environment..."
	$(DOCKER_COMPOSE_PROD) logs -f

# Help
help:
	@echo "Available commands:"
	@echo "  build             - Build the application"
	@echo "  run               - Run the application locally (uses .env.dev)"
	@echo "  test              - Run tests"
	@echo "  coverage          - Run tests with coverage"
	@echo "  clean             - Clean build artifacts"
	@echo "  tidy              - Tidy go.mod and go.sum"
	@echo "  wire              - Generate Wire dependency injection code"
	@echo "  migrate-up        - Apply all database migrations to dev DB (uses .env.dev)"
	@echo "  migrate-down      - Revert the last migration on dev DB (uses .env.dev)"
	@echo "  migrate-create NAME=<migration_name> - Create new migration files (e.g., make migrate-create NAME=add_new_feature)"
	@echo "  docker-dev-up     - Start development environment with Docker Compose (uses .env.dev)"
	@echo "  docker-dev-down   - Stop development environment"
	@echo "  docker-dev-logs   - Show logs for development environment"
	@echo "  docker-prod-up    - Start production environment with Docker Compose (uses .env.prod)"
	@echo "  docker-prod-down  - Stop production environment"
	@echo "  docker-prod-logs  - Show logs for production environment"