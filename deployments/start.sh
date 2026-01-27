#!/bin/sh
set -e

echo "=== Starting GemFactory ==="

# Wait for postgres
echo "Waiting for PostgreSQL..."
until pg_isready -d "$DB_DSN"; do
  echo "PostgreSQL is not ready - waiting..."
  sleep 2
done

# Migrations
echo "Applying migrations..."
for f in /app/migrations/*.up.sql; do
  echo "Applying migration $f..."
  psql -d "$DB_DSN" -f "$f" || echo "Error applying $f (might already exist)"
done

# Check tables
echo "Verifying tables..."
psql -d "$DB_DSN" -c "
SELECT 'config' as table_name, COUNT(*) as record_count FROM gemfactory.config
UNION ALL
SELECT 'artists' as table_name, COUNT(*) as record_count FROM gemfactory.artists;
" || echo "Error verifying tables"

# Init dirs
echo "Initializing directories..."
mkdir -p /app/data /app/logs

# Permissions check
echo "Verifying permissions..."
ls -la /app/data || echo "Failed to list data directory"
ls -la /app/logs || echo "Failed to list logs directory"

# Check write access
echo "Testing write access..."
if touch /app/logs/test.log 2>/dev/null; then
    echo "✓ Write access to /app/logs confirmed"
    rm -f /app/logs/test.log
else
    echo "✗ Write access to /app/logs failed"
fi

if touch /app/data/test.txt 2>/dev/null; then
    echo "✓ Write access to /app/data confirmed"
    rm -f /app/data/test.txt
else
    echo "✗ Write access to /app/data failed"
fi

# Run app
echo "Starting application binary..."
exec ./gemfactory