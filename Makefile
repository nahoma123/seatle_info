# File: Makefile

.PHONY: all build run test coverage clean tidy help wire \
            migrate-up migrate-down migrate-create \
            dev-db-up dev-db-down logs-dev-db run-dev-app seed-dev-db \
            prod-env-up prod-env-down logs-prod-env migrate-up-prod migrate-down-prod

# Variables
APP_NAME=seattle_info_backend
CMD_DIR=./cmd/server
BUILD_DIR=./build
MIGRATE_PATH=./migrations
# MIGRATE_DSN_ENV_VAR=DB_SOURCE # Less critical now, sourced from files

ENV_DEV_FILE=.env.development
ENV_PROD_FILE=.env.production
DOCKER_COMPOSE_DEV=docker-compose -f docker-compose.dev.yml --env-file $(ENV_DEV_FILE)
DOCKER_COMPOSE_PROD=docker-compose -f docker-compose.yml --env-file $(ENV_PROD_FILE)

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

# Run the application (uses local go, intended for quick tests, not primary dev workflow)
run:
	@echo "Running $(APP_NAME) (direct go run, for quick tests)..."
	$(GO_RUN) $(CMD_DIR)/main.go

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

# Database Migrations (for Development Environment by default)
migrate-up: $(ENV_DEV_FILE)
	@echo "Applying migrations to development database..."
	@echo "Ensure your $(ENV_DEV_FILE) has correct DB_SOURCE for localhost."
	set -a; source $(ENV_DEV_FILE); set +a; \
	migrate -database "$${DB_SOURCE}" -path $(MIGRATE_PATH) up

migrate-down: $(ENV_DEV_FILE)
	@echo "Reverting last migration on development database..."
	@echo "Ensure your $(ENV_DEV_FILE) has correct DB_SOURCE for localhost."
	set -a; source $(ENV_DEV_FILE); set +a; \
	migrate -database "$${DB_SOURCE}" -path $(MIGRATE_PATH) down 1

migrate-create: NAME?=new_migration
	@echo "Creating migration: $(NAME)..."
	migrate create -ext sql -dir $(MIGRATE_PATH) -seq $(NAME)
	@echo "Migration files created in $(MIGRATE_PATH)"

# Development Database Environment (PostgreSQL using Docker)
dev-db-up: $(ENV_DEV_FILE)
	@echo "Starting development PostgreSQL container..."
	$(DOCKER_COMPOSE_DEV) up -d postgres_db --build
	@echo "Development PostgreSQL started."

dev-db-down:
	@echo "Stopping development PostgreSQL container..."
	$(DOCKER_COMPOSE_DEV) down # This will stop all services in docker-compose.dev.yml, which is postgres_db
	@echo "Development PostgreSQL stopped."

logs-dev-db:
	@echo "Showing logs for development PostgreSQL container..."
	$(DOCKER_COMPOSE_DEV) logs -f postgres_db

# Run Go Application for Development (connects to dev-db)
run-dev-app: $(ENV_DEV_FILE) dev-db-up
	@echo "Running $(APP_NAME) for development (connecting to PostgreSQL on Docker)..."
	@echo "Ensure your $(ENV_DEV_FILE) is configured correctly."
	@# Source .env.development and run the Go application
	set -a; source $(ENV_DEV_FILE); set +a; \
	$(GO_RUN) $(CMD_DIR)/main.go

# Seed Development Database
seed-dev-db: $(ENV_DEV_FILE) dev-db-up
	@echo "Seeding development database..."
	@echo "Ensure development database is running. If you see errors, run 'make dev-db-up' first."
	@# Extract DB_USER and DB_NAME from .env.development for psql command
	set -a; source $(ENV_DEV_FILE); set +a; \
	$(DOCKER_COMPOSE_DEV) exec -T postgres_db psql -U "$${DB_USER}" -d "$${DB_NAME}" < $(MIGRATE_PATH)/seed.sql
	@echo "Development database seeded."

# Production-like Docker Environment (Go App + PostgreSQL in Docker)
prod-env-up: $(ENV_PROD_FILE)
	@echo "Starting production-like environment with Docker Compose..."
	@echo "Ensure your $(ENV_PROD_FILE) is configured correctly."
	$(DOCKER_COMPOSE_PROD) up -d --build
	@echo "Production-like environment started."

prod-env-down:
	@echo "Stopping production-like environment..."
	$(DOCKER_COMPOSE_PROD) down
	@echo "Production-like environment stopped."

logs-prod-env:
	@echo "Showing logs for production-like environment..."
	$(DOCKER_COMPOSE_PROD) logs -f

# Migrations for Production-like Environment
migrate-up-prod: $(ENV_PROD_FILE)
	@echo "Applying migrations to production-like database..."
	@echo "Ensure your $(ENV_PROD_FILE) has DB_SOURCE_PROD configured to connect to the 'prod-env-up' database (e.g., localhost:exposed_port)."
	set -a; source $(ENV_PROD_FILE); set +a; \
	migrate -database "$${DB_SOURCE_PROD}" -path $(MIGRATE_PATH) up
	@echo "Migrations applied. Note: DB_SOURCE_PROD should be set in $(ENV_PROD_FILE) pointing to the exposed prod DB port if different from dev."

migrate-down-prod: $(ENV_PROD_FILE)
	@echo "Reverting last migration on production-like database..."
	@echo "Ensure your $(ENV_PROD_FILE) has DB_SOURCE_PROD configured."
	set -a; source $(ENV_PROD_FILE); set +a; \
	migrate -database "$${DB_SOURCE_PROD}" -path $(MIGRATE_PATH) down 1

# Check for .env files and guide user if missing
$(ENV_DEV_FILE):
	@echo "Development environment file ($(ENV_DEV_FILE)) not found."
	@echo "Please create it, for example by copying from .env.example: cp .env.example $(ENV_DEV_FILE)"
	@echo "Then, configure it for your local development."
	@exit 1

$(ENV_PROD_FILE):
	@echo "Production environment file ($(ENV_PROD_FILE)) not found."
	@echo "Please create it, for example by copying from .env.example: cp .env.example $(ENV_PROD_FILE)"
	@echo "Then, configure it for your production-like environment."
	@exit 1

# Help
help:
	@echo "Available commands:"
	@echo ""
	@echo "General Development:"
	@echo "  build          - Build the Go application binary"
	@echo "  run            - Run the Go application directly (for quick tests, uses local env)"
	@echo "  run-dev-app    - Run the Go application locally (connects to dev DB, uses $(ENV_DEV_FILE))"
	@echo "  test           - Run tests"
	@echo "  coverage       - Run tests with coverage"
	@echo "  clean          - Clean build artifacts"
	@echo "  tidy           - Tidy go.mod and go.sum"
	@echo "  wire           - Generate Wire dependency injection code"
	@echo ""
	@echo "Development Database (PostgreSQL in Docker via $(ENV_DEV_FILE)):"
	@echo "  dev-db-up      - Start the development PostgreSQL container"
	@echo "  dev-db-down    - Stop the development PostgreSQL container"
	@echo "  logs-dev-db    - Show logs for the development PostgreSQL container"
	@echo "  seed-dev-db    - Populate the development database with sample data from $(MIGRATE_PATH)/seed.sql"
	@echo ""
	@echo "Development Migrations (targets DB specified in $(ENV_DEV_FILE) via DB_SOURCE):"
	@echo "  migrate-up     - Apply all database migrations"
	@echo "  migrate-down   - Revert the last database migration"
	@echo "  migrate-create NAME=<migration_name> - Create new migration files"
	@echo ""
	@echo "Production-like Environment (Go App + PostgreSQL in Docker via $(ENV_PROD_FILE)):"
	@echo "  prod-env-up    - Start all services (app & db) for a production-like environment"
	@echo "  prod-env-down  - Stop all services in the production-like environment"
	@echo "  logs-prod-env  - Show logs for all services in the production-like environment"
	@echo ""
	@echo "Production-like Migrations (targets DB specified in $(ENV_PROD_FILE) via DB_SOURCE_PROD):"
	@echo "  migrate-up-prod   - Apply all migrations to the production-like database"
	@echo "  migrate-down-prod - Revert the last migration on the production-like database"