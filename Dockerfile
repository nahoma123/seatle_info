# File: Dockerfile

# --- Builder Stage ---
FROM golang:1.23-alpine AS builder

# Set working directory
WORKDIR /app

# --- NEW: Install curl so we can download the migrate tool ---
RUN apk --no-cache add curl

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy the entire project
COPY . .

# Build the application
# The main.go and wire_gen.go are in cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -v -o /app/server ./cmd/server

# --- NEW: Download and extract the migrate CLI tool ---
# We download a specific version for reproducible builds.
RUN curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.1/migrate.linux-amd64.tar.gz | tar xvz
# This will extract the 'migrate' executable to /app/migrate

# --- Release Stage ---
FROM alpine:latest

# Install CA certificates for HTTPS calls, and timezone data
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# --- NEW: Copy the migrate CLI from the builder stage ---
# We place it in /usr/local/bin, which is in the system's $PATH,
# so the 'migrate' command can be found.
COPY --from=builder /app/migrate /usr/local/bin/

# Copy the built application binary from the builder stage
COPY --from=builder /app/server /app/server

# --- NEW & IMPORTANT: Copy the migration files ---
# Your deploy script runs migrations from the path "/migrations".
# This line copies your local 'migrations' folder to that exact path inside the container.
COPY migrations /migrations

# Set the entrypoint for the container (this is for your main app)
ENTRYPOINT ["/app/server"]