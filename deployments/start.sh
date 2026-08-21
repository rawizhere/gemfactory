#!/bin/sh
set -e

echo "=== Starting GemFactory ==="

# Wait for PostgreSQL
echo "Waiting for PostgreSQL..."
until pg_isready -d "$DB_DSN"; do
  echo "PostgreSQL is not ready - waiting..."
  sleep 2
done

# Apply migrations
echo "Applying migrations..."
for f in /app/migrations/*.up.sql; do
  if [ -f "$f" ]; then
    echo "Applying migration $f..."
    psql -d "$DB_DSN" -f "$f" || echo "Warning: error applying $f (might already exist)"
  fi
done

# Ensure app directories exist
mkdir -p /app/data /app/logs

# Start application binary
echo "Starting application binary..."
exec ./gemfactory