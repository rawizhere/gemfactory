-- Remove config rows that are now read from environment variables only.
-- BOT_TOKEN and ADMIN_USERNAME are kept: they remain a fallback when env is empty.
DELETE FROM gemfactory.config WHERE key IN (
    'RELEASE_CHECK_INTERVAL', 'RATE_LIMIT_REQUESTS', 'RATE_LIMIT_WINDOW', 'SCRAPER_DELAY',
    'LOG_LEVEL', 'TIMEZONE', 'APP_DATA_DIR', 'DB_DSN', 'WEB_PORT', 'WEB_ENABLED',
    'HEALTH_PORT', 'HEALTH_CHECK_ENABLED'
);
