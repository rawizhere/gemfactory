#!/bin/sh
set -e

echo "=== Starting GemFactory ==="

# Wait for PostgreSQL
echo "Waiting for PostgreSQL..."
until pg_isready -d "$DB_DSN"; do
  echo "PostgreSQL is not ready - waiting..."
  sleep 2
done

# Migrations are applied by the application itself (embedded goose) on startup.

# Ensure app directories exist
mkdir -p /app/data /app/logs

# Start application binary
echo "Starting application binary..."
exec ./gemfactory