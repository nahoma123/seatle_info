# File: Dockerfile

# --- STAGE 1: Builder ---
# This stage builds the Go application and downloads necessary tools.
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install curl, which is needed to download the migrate tool.
RUN apk --no-cache add curl

# Copy dependency files first to leverage Docker's layer caching.
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy the rest of the source code.
COPY . .

# Build the main application binary.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -v -o /app/server ./cmd/server

# Download and extract the golang-migrate CLI tool.
RUN curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.1/migrate.linux-amd64.tar.gz | tar xvz
# This extracts the 'migrate' executable to /app/migrate


# --- STAGE 2: Release ---
# This stage creates the final, minimal production image.
FROM alpine:latest

# Install necessary certificates for making HTTPS requests.
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the compiled application binary from the builder stage.
COPY --from=builder /app/server /app/server

# Copy the migrate tool from the builder stage into a standard PATH directory.
COPY --from=builder /app/migrate /usr/local/bin/

# Copy the migration SQL files into the location expected by our entrypoint script.
COPY migrations /migrations

# Copy the new entrypoint script and make it executable.
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Set the new script as the container's entrypoint.
ENTRYPOINT ["docker-entrypoint.sh"]

# Define the default command to pass to the entrypoint script.
# This will run the web server by default if no other command is specified.
CMD ["/app/server"]