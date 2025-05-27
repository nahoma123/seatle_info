# File: Dockerfile

# --- Builder Stage ---
    FROM golang:1.21-alpine AS builder

    # Set working directory
    WORKDIR /app
    
    # Copy go.mod and go.sum first to leverage Docker cache
    COPY go.mod go.sum ./
    RUN go mod download && go mod verify
    
    # Copy the entire project
    COPY . .
    
    # Generate Wire code (if not already committed)
    # RUN cd cmd/server && go generate ./...
    # Or, more robustly, ensure wire_gen.go is committed or copy it specifically if generated outside.
    # For now, assume wire_gen.go might be generated locally and copied or that make wire is run before build.
    
    # Build the application
    # The main.go and wire_gen.go are in cmd/server
    RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -v -o /app/server ./cmd/server
    
    # --- Release Stage ---
    FROM alpine:latest
    
    # Install CA certificates for HTTPS calls, and timezone data
    RUN apk --no-cache add ca-certificates tzdata
    
    WORKDIR /app
    
    # Copy the built binary from the builder stage
    COPY --from=builder /app/server /app/server
    
    # Copy migrations (optional, if you want to run migrations from within the container,
    # but usually migrations are run as a separate step against the DB)
    # COPY migrations ./migrations
    
    # Copy .env.example (optional, for reference, actual .env should be injected)
    # COPY .env.example .env.example
    
    # Expose the application port (as defined by SERVER_PORT in .env)
    # This is informational; the actual port mapping is done in docker-compose or `docker run -p`
    # EXPOSE 8080 (Hardcoding for now, ideally from an ARG)
    
    # Set the entrypoint for the container
    ENTRYPOINT ["/app/server"]
    
    # Command to run (can be overridden)
    # CMD ["/app/server"] # ENTRYPOINT is usually sufficient if it's just the binary