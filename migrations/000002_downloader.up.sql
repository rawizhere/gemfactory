-- Downloader: yt-dlp cookies storage per domain.
CREATE TABLE IF NOT EXISTS gemfactory.cookies (
    cookie_id SERIAL PRIMARY KEY,
    domain VARCHAR(255) NOT NULL UNIQUE,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
