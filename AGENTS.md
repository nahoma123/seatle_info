Absolutely. Based on the provided Makefile, docker-compose.dev.yml, docker-compose.yml, and the GitHub Actions workflow (.deploy.yml), I can create a much more accurate and helpful AGENTS.md file.

This new version will reflect the project's tooling, deployment strategy, and development environment. It will be a practical guide for any AI or human developer joining the project.

AGENTS.md - Instructions for AI Agents & Developers

This document provides essential guidelines for contributing to the Seattle Info Backend project. Adhering to these instructions ensures consistency, quality, and efficient development, especially for CI/CD and deployment.

1. Project Overview

Goal: To develop a mobile-first community information platform named "Seattle Info," dedicated to serving the Habesha community in Seattle.

Technology Stack:

Language: Go (Golang)

Web Framework: Gin-Gonic

Database: PostgreSQL with PostGIS extension

ORM: GORM

Dependency Injection: Google Wire

Configuration: Viper & godotenv (.env files)

Logging: Zap (structured logging)

Authentication: JWT (email/password) & OAuth 2.0 (Google, Apple)

Background Jobs: robfig/cron

Migrations: golang-migrate/migrate

Containerization: Docker & Docker Compose

CI/CD: GitHub Actions for building images and deploying to EC2.

2. Local Development Environment

The recommended local setup uses Docker Compose to manage services like the database and go run on your local machine for the application itself.

2.1. Prerequisites

Go: Version 1.21 or newer (see go.mod).

Docker & Docker Compose: Required for running services.

wire CLI: Install via go install github.com/google/wire/cmd/wire@latest.

migrate CLI: (Optional but recommended for local use) Install from golang-migrate/migrate.

2.2. Initial Setup

Clone the Repository.

Create Environment File: Copy .env.example to a new file named .env.

Generated bash
cp .env.example .env


Configure .env: Open .env and fill in the values. The default values for DB_USER, DB_PASSWORD, DB_NAME, and DB_PORT should work seamlessly with docker-compose.dev.yml. You must set secrets like JWT_SECRET_KEY and OAuth credentials.

Start Services: Use Docker Compose to start the PostgreSQL database and pgAdmin.

Generated bash
docker-compose -f docker-compose.dev.yml up -d
IGNORE_WHEN_COPYING_START
content_copy
download
Use code with caution.
Bash
IGNORE_WHEN_COPYING_END

The database will be available on localhost:5432 (or your configured DB_PORT).

You can access pgAdmin at http://localhost:5050.

2.3. Running the Application Locally

Generate DI Code: After any dependency changes in wire.go, you must run:

Generated bash
wire ./cmd/server
IGNORE_WHEN_COPYING_START
content_copy
download
Use code with caution.
Bash
IGNORE_WHEN_COPYING_END

This generates cmd/server/wire_gen.go. This step is crucial and must be done before building or running.

Run the Go application:

Generated bash
go run ./cmd/server/main.go ./cmd/server/wire_gen.go
IGNORE_WHEN_COPYING_START
content_copy
download
Use code with caution.
Bash
IGNORE_WHEN_COPYING_END

The run command in the Makefile is currently minimal and may not correctly export environment variables. The command above is the most reliable way to run locally.

2.4. Running Database Migrations Locally

The docker-compose.dev.yml file provides a migrate service for running migrations against the development database.

Ensure your .env file has the DB_SOURCE variable correctly set, pointing to the Docker container (postgres_db):

Generated env
# In .env
DB_SOURCE="postgresql://your_db_user:your_db_password@postgres_db:5432/seattle_info_db?sslmode=disable"
IGNORE_WHEN_COPYING_START
content_copy
download
Use code with caution.
Env
IGNORE_WHEN_COPYING_END

Note: When running the migrate service from Docker Compose, the DB host is postgres_db, not localhost.

Create a new migration:

Generated bash
docker-compose -f docker-compose.dev.yml run --rm migrate create -ext sql -dir /migrations -seq your_migration_name
IGNORE_WHEN_COPYING_START
content_copy
download
Use code with caution.
Bash
IGNORE_WHEN_COPYING_END

Apply migrations:

Generated bash
docker-compose -f docker-compose.dev.yml run --rm migrate up
IGNORE_WHEN_COPYING_START
content_copy
download
Use code with caution.
Bash
IGNORE_WHEN_COPYING_END

Revert the last migration:

Generated bash
docker-compose -f docker-compose.dev.yml run --rm migrate down 1
IGNORE_WHEN_COPYING_START
content_copy
download
Use code with caution.
Bash
IGNORE_WHEN_COPYING_END
3. Development Guidelines
3.1. Code Style & Conventions

Formatting: All Go code must be formatted with go fmt ./....

Error Handling: Use the custom error framework in internal/common/errors.go. Handlers must use common.RespondWithError(c, err).

Logging: Use the injected *zap.Logger for structured, contextual logging. Provide relevant IDs and data.

API Documentation: Keep API_DOCUMENTATION.md up-to-date with any changes to endpoints.

3.2. Adding a Feature (Typical Workflow)

Model: Define or update GORM models and API DTOs in the relevant internal/{domain}/model.go.

Repository: Add or update methods in the Repository interface and its GORM implementation in internal/{domain}/repository.go.

Service: Add or update methods in the Service interface and its concrete implementation in internal/{domain}/service.go. This is where business logic resides.

Handler: Add or update Gin handlers in internal/{domain}/handler.go to expose the new service functionality via an API endpoint.

Routing: Register the new routes in the handler's RegisterRoutes method. Ensure it's called from internal/app/server.go.

Dependency Injection: If new services or repositories are created, add their providers (New...) to the appropriate wire.NewSet in cmd/server/wire.go.

Regenerate Code: Run wire ./cmd/server to update wire_gen.go.

Test: Run the application and test the new endpoint manually or with an API client.

4. Deployment (CI/CD)

The project uses a GitHub Actions workflow defined in .github/workflows/deploy.yml to automate deployment to an EC2 instance.

4.1. Workflow Summary

Trigger: A push to the main branch.

Job 1: build-and-push-image

Builds a production-ready Docker image of the Go application.

Pushes the image to GitHub Container Registry (GHCR) with latest and commit sha tags.

Job 2: deploy-to-ec2

Runs after the image is successfully pushed.

Uses SSH to connect to the production EC2 server.

Executes a remote script on the server which does the following:

Navigates to the project directory (~/seatle_info).

Creates persistent directories if they don't exist (e.g., ./config, ./images/listings).

Writes production secrets (from GitHub Secrets) into a .env file on the server.

Logs into GHCR using a deploy token.

Pulls the latest application image using docker compose pull.

Restarts the application stack using docker compose up -d.

Runs database migrations using docker compose run --rm migrate.

Prunes old Docker images to save space.

4.2. Production Environment on EC2

The application runs via the docker-compose.yml file on the server.

The app service uses the Docker image built by the CI/CD pipeline.

The postgres_db service uses a persistent Docker volume (postgres_data_prod) to store database data.

The migrate service in the production docker-compose.yml is specifically configured to run migrations on startup: command: ["migrate", "up"]. This seems to be an error in the deploy script which runs it separately. The deploy script's docker compose run --rm migrate is the primary method.

Secret Management: All secrets (database passwords, API keys, SSH keys) are managed as GitHub Encrypted Secrets and injected into the environment at deploy time. NEVER commit secrets to the repository.

5. Key Project Structure

cmd/server/: Main application entry point (main.go) and Wire DI setup (wire.go).

internal/: Core application logic.

internal/app/: Server setup and application lifecycle.

internal/{auth,user,category,listing}/: Domain-specific packages (Handler, Service, Repository, Model).

internal/common/, internal/platform/, internal/middleware/, internal/jobs/: Shared and platform-level code.

migrations/: Database migration SQL scripts.

API_DOCUMENTATION.md: API reference.

.github/workflows/deploy.yml: The CI/CD deployment pipeline.

Dockerfile: Defines the production Docker image.

docker-compose.yml: Production Docker Compose setup.

docker-compose.dev.yml: Local development Docker Compose setup.

.env.example: Template for environment variables.

Makefile: Contains common development tasks (currently minimal).

This document is a living guide. Please update it as the project evolves.