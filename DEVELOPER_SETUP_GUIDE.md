# Developer Setup Guide

## Introduction

Welcome to the Seattle Info Backend project! This application serves as a backend API for a listings platform, providing functionalities for managing users, categories, listings, and notifications, with a powerful search capability backed by Elasticsearch.

This guide will help you set up your local development environment.

## Prerequisites

Ensure you have the following installed on your system:

*   **Go:** Version 1.21 or later.
*   **Docker:** Latest stable version.
*   **Docker Compose:** Latest stable version (usually included with Docker Desktop).
*   **Git:** Latest stable version.

## Getting Started

Follow these steps to get the application running locally:

### 1. Clone Repository

Clone the project repository to your local machine:

```bash
git clone <repository-url> # Replace <repository-url> with the actual Git repository URL
cd seattle_info_backend # Or your project's root directory name
```

### 2. Environment Configuration

The application uses environment variables for configuration, managed via a `.env` file.

*   **Copy the example environment file:**
    ```bash
    cp .env.example .env
    ```
*   **Review and update `.env`:**
    Open the newly created `.env` file and review the following key variables. Update them if necessary for your local setup, though the defaults are generally suitable for local development using the provided Docker Compose configuration.

    *   **PostgreSQL Configuration:**
        *   `DB_HOST`: Hostname for PostgreSQL (default: `localhost` if app runs locally, `postgres_db` if app runs in Docker).
        *   `DB_PORT`: Port for PostgreSQL (default: `5432`).
        *   `DB_USER`: Database username (default: `your_db_user`).
        *   `DB_PASSWORD`: Database password (default: `your_db_password`).
        *   `DB_NAME`: Database name (default: `seattle_info_db`).
        *   `DB_SOURCE`: The full DSN URL for `golang-migrate`. Ensure this matches the above credentials and the service name `postgres_db` if running migrations against the Docker container. Example: `postgresql://your_db_user:your_db_password@localhost:5432/seattle_info_db?sslmode=disable` (if app/migrate runs locally) or `postgresql://your_db_user:your_db_password@postgres_db:5432/seattle_info_db?sslmode=disable` (if migrate runs inside Docker network). The `.env.example` usually provides a good default for local development.

    *   **Elasticsearch Configuration:**
        *   `ELASTICSEARCH_URL`: URL for the Elasticsearch service.
            *   If running the Go application **locally** (e.g., `go run ./cmd/server/main.go`) and Elasticsearch is running in Docker (as per `docker-compose.dev.yml`), this should be `http://localhost:9200`.
            *   If running the Go application **inside Docker** via a potential `app` service in `docker-compose.dev.yml`, this would be `http://elasticsearch:9200` (service name).
            *   The default in `.env.example` is typically `http://localhost:9200`.

    *   **Server Configuration:**
        *   `GIN_MODE`: Gin framework mode (e.g., `debug`, `release`). `debug` is recommended for local development.
        *   `SERVER_PORT`: Port on which the Go application will listen (default: `8080`).

    *   **Authentication:**
        *   `JWT_SECRET_KEY`: A secret key for signing JWTs. For Firebase integration (used for token verification), this refers to the service account key JSON file path. Generate a strong secret key for other JWT purposes if any.
        *   `FIREBASE_SERVICE_ACCOUNT_KEY_PATH`: Path to your Firebase service account JSON key file. This is critical for Firebase Admin SDK initialization and user authentication. Example: `./config/your-firebase-adminsdk.json`.
        *   `FIREBASE_PROJECT_ID`: Your Firebase Project ID.

### 3. Start Services (PostgreSQL & Elasticsearch)

The project uses Docker Compose to manage external services like PostgreSQL and Elasticsearch for development.

*   **Start the services in detached mode:**
    ```bash
    docker-compose -f docker-compose.dev.yml up -d
    ```
    This command will:
    *   Pull the necessary Docker images for PostgreSQL (with PostGIS) and Elasticsearch if they are not already present locally.
    *   Create and start containers for these services.
    *   PostgreSQL will be available on `localhost:<DB_PORT>` (e.g., `localhost:5432`).
    *   Elasticsearch will be available on `localhost:9200` (HTTP) and `localhost:9300` (Transport).

    You can check the status of the containers using `docker-compose -f docker-compose.dev.yml ps`.

### 4. Install Go Dependencies

Fetch and install the Go module dependencies defined in `go.mod`:

```bash
go mod tidy
# or
# go mod download
```
`go mod tidy` is generally preferred as it also removes any unused dependencies.

## Running the Application

Once the prerequisites are met, environment configured, services running, and dependencies installed:

*   **Run the Go application:**
    ```bash
    go run ./cmd/server/main.go
    ```
*   The application will start, and by default, it will be accessible at `http://localhost:8080` (or the port specified in your `.env` file).

## Running Tests

*   **Unit and Integration Tests:**
    To run all tests (unit and integration tests that may require DB/ES):
    ```bash
    go test ./...
    ```
    Or, to run integration tests which are typically skipped unless explicitly enabled:
    ```bash
    RUN_INTEGRATION_TESTS=true go test ./...
    ```
*   **Prerequisites for Tests:** Ensure that the Docker services (PostgreSQL and Elasticsearch) are running as defined in `docker-compose.dev.yml` before executing tests that rely on them.

## Database Migrations

Database schema migrations are managed using `golang-migrate`. The migration files are located in the `/migrations` directory.

*   **To run migrations:**
    Migrations are typically run using the `migrate` Docker container defined in `docker-compose.dev.yml`.
    Ensure your `DB_SOURCE` variable in `.env` is correctly set up for the `migrate` tool (it should point to the `postgres_db` service name if running `migrate` via Docker Compose against the Dockerized DB).

    Example command to apply all `up` migrations:
    ```bash
    docker-compose -f docker-compose.dev.yml run --rm migrate -path /migrations -database "$DB_SOURCE" up
    ```
    To apply down migrations, or specific versions, refer to `golang-migrate` CLI documentation.
    *   Make sure the `$DB_SOURCE` environment variable is available to this command, or replace it directly with the DSN string from your `.env` file (quoted).

## Elasticsearch Integration Notes

*   **Purpose:** Elasticsearch is integrated into the application primarily for powering the search functionality for listings, providing more advanced search capabilities than direct database queries.
*   **Automatic Index Creation:** The application is configured to automatically create the `listings` index with the required mapping in Elasticsearch when it starts up, if the index does not already exist. This is handled by logic in `internal/platform/elasticsearch/index.go`.
*   **Service Configuration:** The Elasticsearch service itself is defined in `docker-compose.dev.yml`. You can view its configuration (e.g., image version, ports, volumes) there.
*   **Debugging/Development (Kibana):** For more advanced Elasticsearch development, querying, or debugging, you might find it useful to have Kibana. You can optionally add a Kibana service to your `docker-compose.dev.yml` file and link it to the Elasticsearch service. Refer to the official Elastic documentation for setting up Kibana with Docker.

## Project Structure (Brief Overview)

*   `cmd/server/`: Contains the `main.go` for the primary application server and Wire setup (`wire.go`, `wire_gen.go`).
*   `internal/`: Contains all the core application logic, separated by domain/feature (e.g., `listing/`, `user/`, `auth/`) and platform concerns (e.g., `platform/database/`, `platform/elasticsearch/`).
    *   `internal/app/`: Core application setup, server definition, and HTTP routing.
    *   `internal/config/`: Configuration loading.
    *   `internal/middleware/`: HTTP middlewares.
    *   `internal/jobs/`: Background job definitions.
*   `migrations/`: Contains SQL migration files for `golang-migrate`.
*   `pkg/`: Shared utility packages that are not specific to this application's internal logic (e.g., could be `database` helpers, `logging` wrappers if they were more generic).
*   `docker-compose.dev.yml`: Docker Compose configuration for development services.
*   `.env.example`: Example environment file.

This structure aims to follow standard Go project layout conventions.
