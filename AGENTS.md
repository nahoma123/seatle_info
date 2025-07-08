# AGENTS.md - Instructions for AI Agents

This document provides guidelines for AI agents working on the Seattle Info project.

## 1. Project Overview

The Seattle Info project is a Go-based backend application that provides information services. Key features include user authentication, listings, categories, and notifications.

## 2. Getting Started

### 2.1. Go Version
- This project uses Go 1.23 (as specified in `go.mod` and `Dockerfile`). Ensure your environment matches this version.

### 2.2. Dependencies
- Dependencies are managed using Go Modules (`go.mod`, `go.sum`). Use `go mod tidy` to ensure dependencies are consistent after making changes to `go.mod`.
- Vendoring is not currently used.

### 2.3. Environment Variables
- Copy `.env.example` to `.env` and populate it with the necessary configuration values for your local setup.
- Key variables include database connection strings (`DB_SOURCE`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_HOST`, `DB_PORT`), JWT secrets (`JWT_SECRET_KEY`), and Firebase credentials.
- The `DB_SOURCE` variable is particularly important for database operations, including migrations. For local development against the Dockerized Postgres, it might look like: `postgresql://your_user:your_password@localhost:5432/seattle_info_db?sslmode=disable`.

### 2.4. Running the Application
- **Using Docker (Recommended for Development with Database):**
  - This starts the PostgreSQL database and other services like pgAdmin.
  - Run: `docker-compose -f docker-compose.dev.yml up --build -d postgres_db` (to start DB in background)
  - Then, run the Go application locally (see next section), connecting to this database.
  - To run the Go application itself inside Docker (optional, useful for full containerized environment):
    - Uncomment and configure the `app` service in `docker-compose.dev.yml`.
    - Then run: `docker-compose -f docker-compose.dev.yml up --build app`
- **Running Locally (Directly with Go, connecting to Dockerized DB):**
  - Ensure PostgreSQL (via Docker as above) is running and accessible.
  - Source your environment variables from `.env`. One way to do this, if your shell supports it: `export $(cat .env | xargs)`
  - Run the application: `go run ./cmd/server/main.go ./cmd/server/wire_gen.go`
    (Note: `wire_gen.go` might need to be generated if not present or if `wire.go` changed, see section on Code Generation).

## 3. Development Guidelines

### 3.1. Code Style & Conventions
- **Formatting:** Adhere to standard Go formatting. Run `go fmt ./...` before committing changes.
- **Linting:** While no specific linter is strictly enforced project-wide yet, it is highly recommended to use `golangci-lint`.
    - Install `golangci-lint` (see [official installation guide](https://golangci-lint.run/usage/install/)).
    - Run `golangci-lint run ./...` from the project root to check for issues.
    - For consistent linting rules, consider adding a `.golangci.yml` configuration file to the project root.
- **Error Handling:**
    - Utilize the custom error types and functions defined in `internal/common/errors.go`.
    - Return `*common.APIError` from HTTP handlers where appropriate to ensure consistent JSON error responses.
    - For new error types, consider if they fit within the existing `common.APIError` framework or if a new standard error should be added to `internal/common/errors.go`.
- **Logging:**
    - Use the logger provided via dependency injection (based on `internal/platform/logger/zap.go`). This is typically an `*zap.Logger` instance.
    - Provide structured logging with relevant context (e.g., `logger.Info("message", zap.String("key", "value"))`).
- **Comments:** Write clear and concise comments for public functions, structs, interfaces, and complex logic sections. Explain *why* something is done, not just *what* it does if the code is already clear.

### 3.2. API Design
- Follow RESTful principles for API design where applicable.
- Refer to `API_DOCUMENTATION.md` for existing API endpoints, request/response formats, and conventions.
- When adding or modifying API endpoints, update `API_DOCUMENTATION.md` accordingly.

### 3.3. Code Generation
- This project uses `google/wire` for dependency injection, primarily configured in `cmd/server/wire.go`.
- If you modify dependencies that are part of the Wire setup (e.g., in `wire.go` or the constructors it uses), you will need to regenerate `cmd/server/wire_gen.go`.
- To regenerate Wire code, run the `go generate` command. It's good practice to specify the file containing the `//go:generate` directive:
  ```bash
  go generate ./cmd/server/main.go
  ```
  Or, more broadly, to run all generators in the project (if others exist):
  ```bash
  go generate ./...
  ```

## 4. Database

### 4.1. Migrations
- Database migrations are managed using `golang-migrate/migrate`.
- Migration files (SQL) are located in the `migrations/` directory.
- **To create a new migration:**
  ```bash
  make migrate-create NAME=your_descriptive_migration_name
  ```
  (This is a presumed `Makefile` target based on `migrations/README.md`. If it doesn't exist, use the `migrate` CLI directly: `migrate create -ext sql -dir migrations -seq your_descriptive_migration_name`)
- **To apply migrations:**
  - Ensure `DB_SOURCE` is correctly set in your environment.
  - **Using local `migrate` CLI** (points to Dockerized DB):
    ```bash
    migrate -database "$DB_SOURCE" -path ./migrations up
    ```
  - **Using Dockerized `migrate` service** (from `docker-compose.dev.yml`, ensure `DB_SOURCE` in `.env` is set for service-to-service connection, e.g., host `postgres_db`):
    ```bash
    docker-compose -f docker-compose.dev.yml run --rm migrate -path /migrations -database "$DB_SOURCE" up
    ```
    *(The `Makefile` might offer simpler targets like `make migrate-up`; prefer those if available and correctly configured.)*
- **To revert the last migration:**
    ```bash
    migrate -database "$DB_SOURCE" -path ./migrations down 1
    ```
    (Or using the Dockerized `migrate` service as above, replacing `up` with `down 1`).
- Always write both `*.up.sql` and `*.down.sql` files for each migration.
- **Important:** Do not edit migration files that have already been applied to shared environments (staging, production). Create a new migration to make further schema changes.

## 5. Testing

### 5.1. Running Tests
- **Run all unit tests:**
  ```bash
  go test $(go list ./... | grep -v /tests/integration)
  ```
  (This command attempts to exclude integration tests. Adjust if needed.)
- **Run unit tests for a specific package:**
  ```bash
  go test ./internal/listing/...
  ```
- **Run all integration tests (`tests/integration`):**
    1.  Ensure your `.env` file is fully configured, especially all `DB_*` variables. The `DB_HOST` for integration tests running locally against the Dockerized DB should be `localhost` (or `127.0.0.1`).
    2.  Start the development database if not already running:
        ```bash
        docker-compose -f docker-compose.dev.yml up -d postgres_db
        ```
    3.  Apply all database migrations (see section 4.1 on Migrations for methods).
    4.  Run the integration tests, ensuring they pick up the environment variables:
        ```bash
        go test ./tests/integration/...
        ```
    5.  To stop the database when done: `docker-compose -f docker-compose.dev.yml down` (or `stop postgres_db`).

### 5.2. Writing Tests
- Write unit tests for new functions, methods, and business logic. Place them in `_test.go` files alongside the code they test (e.g., `listing_service_test.go` for `listing_service.go`).
- Write integration tests for API endpoints and service interactions that involve external dependencies like the database. Place these in the `tests/integration` directory.
- Utilize the `testify/assert` and `testify/require` packages for assertions.
- Strive for good test coverage. Mock dependencies for unit tests where appropriate.

## 6. Committing and Submitting Changes

- Follow conventional commit message guidelines (e.g., a short imperative subject line, optional body).
- Ensure all unit and integration tests pass before submitting changes:
  - Run `go fmt ./...`
  - Run `golangci-lint run ./...` (if installed)
  - Run unit tests.
  - Run integration tests.
- Update any relevant documentation, including `API_DOCUMENTATION.md` and this `AGENTS.md` if guidelines change.

## 7. Key Project Structure

- `cmd/server/`: Main application entry point (`main.go`) and Wire dependency injection setup (`wire.go`, `wire_gen.go`).
- `internal/`: Core application logic, not intended for import by other projects.
  - `internal/app/`: Server setup (HTTP router, middleware registration) and application lifecycle.
  - `internal/auth/`, `internal/category/`, `internal/listing/`, `internal/user/`, `internal/notification/`: Domain-specific packages, typically containing:
    - `handler.go`: HTTP handlers (Gin).
    - `service.go`: Business logic.
    - `repository.go`: Data access logic (GORM).
    - `model.go`: Domain-specific data structures and validation.
    - `interfaces.go`: (Optional) Interfaces for services/repositories.
  - `internal/common/`: Shared utilities, error definitions (`errors.go`), common models, pagination logic.
  - `internal/config/`: Configuration loading (Viper).
  - `internal/domain/`: Core domain types or constants shared across multiple internal packages.
  - `internal/filestorage/`: Services for interacting with file storage (e.g., local, cloud).
  - `internal/firebase/`: Firebase integration services.
  - `internal/jobs/`: Background job definitions (e.g., using `robfig/cron`).
  - `internal/middleware/`: HTTP middlewares (e.g., auth, logging, error handling).
  - `internal/platform/`: Platform-level concerns or wrappers around external libraries.
    - `internal/platform/database/`: Database connection setup (GORM).
    - `internal/platform/crypto/`: Cryptographic utilities.
    - `internal/platform/logger/`: Logger setup (Zap).
    - `internal/platform/geo/`: Geolocation utilities.
  - `internal/shared/`: Shared business logic or services that don't fit neatly into a single domain but are not platform-level.
- `migrations/`: Database migration SQL scripts. Contains a `README.md` with specific instructions for `golang-migrate`.
- `tests/integration/`: Integration tests that require external services like a database.
- `API_DOCUMENTATION.md`: Markdown documentation for the project's API.
- `Dockerfile`: Defines the Docker image for building and running the production application.
- `docker-compose.yml`: Docker Compose configuration for production-like environment.
- `docker-compose.dev.yml`: Docker Compose configuration for local development (e.g., database, pgAdmin).
- `go.mod`, `go.sum`: Go module files.
- `.env.example`: Template for environment variables.
- `Makefile`: Contains common development tasks (current version is minimal, could be expanded).

---
This document is a living guide. Please update it as the project evolves.
```
