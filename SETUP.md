# Project Setup Guide

This guide provides practical steps to set up and run the application for development and production.

## 1. Prerequisites

- Go (1.21+ or as per `go.mod`)
- Docker & Docker Compose
- `make` utility

## 2. Environment Configuration (`.env` files)

You need to create two files in the project root: `.env.dev` (for development) and `.env.prod` (for production). These files are gitignored.

**Important:** In these files, `SERVER_TIMEOUT_SECONDS` and `DB_CONN_MAX_LIFETIME_MINUTES` should be plain numbers (e.g., `30`, `60`).

**`.env.dev` (Example Content):**
```env
GIN_MODE=debug
SERVER_PORT=8080
SERVER_HOST=0.0.0.0
SERVER_TIMEOUT_SECONDS=30

DB_HOST=postgres_db
DB_PORT=5432
DB_USER=seattle_user        # Your dev DB user
DB_PASSWORD=1234567890    # Your dev DB password
DB_NAME=seattle_info_dev    # Your dev DB name
DB_SSL_MODE=disable
DB_TIMEZONE=UTC
DB_MAX_IDLE_CONNS=10
DB_MAX_OPEN_CONNS=100
DB_CONN_MAX_LIFETIME_MINUTES=60
DB_SOURCE=postgresql://seattle_user:1234567890@postgres_db:5432/seattle_info_dev?sslmode=disable # Match user/pass/db_name

# --- Add your other dev settings below ---
# JWT_SECRET_KEY=your_dev_jwt_secret
# FIREBASE_SERVICE_ACCOUNT_KEY_PATH=./config/your-dev-firebase-key.json # Local path
# GOOGLE_CLIENT_ID=your_dev_google_client_id
# ... etc.
```

**`.env.prod` (Example Content - for production server):**
```env
GIN_MODE=release
SERVER_PORT=80
SERVER_HOST=0.0.0.0
SERVER_TIMEOUT_SECONDS=30

DB_HOST=postgres_db
DB_PORT=5432
DB_USER=your_prod_db_user
DB_PASSWORD=your_prod_db_password
DB_NAME=seattle_info_prod
DB_SSL_MODE=require # Recommended for prod
DB_TIMEZONE=UTC
DB_MAX_IDLE_CONNS=10
DB_MAX_OPEN_CONNS=100
DB_CONN_MAX_LIFETIME_MINUTES=60
DB_SOURCE=postgresql://your_prod_db_user:your_prod_db_password@postgres_db:5432/seattle_info_prod?sslmode=require # Match user/pass/db_name

# --- Add your other prod settings below ---
# JWT_SECRET_KEY=your_strong_production_jwt_secret
# FIREBASE_SERVICE_ACCOUNT_KEY_PATH=/app/secret/your-prod-firebase-key.json # Path inside Docker
# GOOGLE_CLIENT_ID=your_prod_google_client_id
# ... etc.
```
*(Remember to replace placeholder values like `your_dev_db_user` with your actual credentials/settings).*

## 3. Development Workflow

**Option A: Run Go App Locally (DB in Docker)**
1.  Ensure `.env.dev` is created and configured.
2.  Start DB: `docker compose -f docker-compose.dev.yml up -d postgres_db` (add `pgadmin` if needed)
3.  Apply migrations: `make migrate-up`
4.  (If needed) Generate Wire code: `make wire`
5.  Run Go app: `make run` (App on `http://localhost:8080` or `SERVER_PORT` from `.env.dev`)
6.  Stop DB: `docker compose -f docker-compose.dev.yml down`

**Option B: Full Dockerized Development (Recommended for Consistency)**
1.  Ensure `.env.dev` is created and configured.
2.  Start services: `make docker-dev-up` (Go app & DB run in Docker, code changes are live-reloaded)
3.  Apply migrations (if not run automatically): `make migrate-up`
4.  View logs: `make docker-dev-logs`
5.  Stop services: `make docker-dev-down`

**Database Migrations (Development):**
- Create: `make migrate-create NAME="your_migration_name"`
- Apply: `make migrate-up`
- Revert last: `make migrate-down`

## 4. Production Workflow (Docker)

1.  Ensure `.env.prod` is configured on your production server.
2.  Ensure secret files (like Firebase key) are present on the host at paths specified in `docker-compose.yml` volumes.
3.  Deploy/Start: `make docker-prod-up`
4.  **Production Migrations**: Must be handled carefully. The `make migrate-up` is for dev. Plan a strategy for production (e.g., dedicated migration job, manual execution).
5.  View logs: `make docker-prod-logs`
6.  Stop: `make docker-prod-down`

## 5. Useful Makefile Targets

- `make help`: Shows all available commands.
- `make test`: Run tests.
- See `Makefile` for more.
The `migrations/README.md` file contains more details on database migrations.
