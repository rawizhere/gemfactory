FROM golang:1.24-alpine AS builder

# Install system dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./

# Download Go modules
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gemfactory cmd/bot/main.go

# Runtime image
FROM alpine:3.21

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata postgresql-client

# Add non-privileged user
RUN adduser -D -s /bin/sh appuser

# Copy binary from builder
COPY --from=builder /app/gemfactory /app/gemfactory

# Copy database migrations
COPY --from=builder /app/migrations /app/migrations

# Copy entrypoint script
COPY deployments/start.sh /app/start.sh

# Initialize data and log directories
RUN mkdir -p /app/data /app/logs

# Set executable permissions and ownership
RUN chmod +x /app/start.sh /app/gemfactory && \
    chown -R appuser:appuser /app && \
    chmod -R 777 /app/logs /app/data

# Switch to application user
USER appuser

# Set working directory
WORKDIR /app

# Expose application port
EXPOSE 8080

# Execute entrypoint
CMD ["/app/start.sh"]