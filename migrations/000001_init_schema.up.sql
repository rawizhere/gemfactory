-- GemFactory Consolidated Initialization Schema
-- This file merges all previous migrations (001-007) into a single baseline.

-- 1. Schema & Environment
CREATE SCHEMA IF NOT EXISTS gemfactory;
SET search_path TO gemfactory, public;

-- 2. Tables
CREATE TABLE IF NOT EXISTS gemfactory.artists (
    artist_id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    gender VARCHAR(10) NOT NULL CHECK (gender IN ('female', 'male', 'mixed')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS gemfactory.releases (
    release_id SERIAL PRIMARY KEY,
    artist_id INTEGER NOT NULL REFERENCES gemfactory.artists(artist_id) ON DELETE CASCADE,
    title VARCHAR(500) NOT NULL,
    title_track VARCHAR(500),
    album_name VARCHAR(500),
    mv VARCHAR(1000),
    date DATE NOT NULL,
    spotify VARCHAR(1000),
    source_url VARCHAR(1000),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS gemfactory.config (
    id SERIAL PRIMARY KEY,
    key VARCHAR(255) NOT NULL UNIQUE,
    value TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Tasks table removed as logic moved to workers

CREATE TABLE IF NOT EXISTS gemfactory.homeworks (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    track_id VARCHAR(255) NOT NULL,
    spotify_id VARCHAR(255) NOT NULL,
    play_count INTEGER NOT NULL DEFAULT 1,
    issued_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    is_completed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, track_id, spotify_id)
);

CREATE TABLE IF NOT EXISTS gemfactory.playlist_tracks (
    id SERIAL PRIMARY KEY,
    spotify_id VARCHAR(255) NOT NULL,
    track_id VARCHAR(255) NOT NULL,
    artist VARCHAR(255) NOT NULL,
    title VARCHAR(500) NOT NULL,
    album VARCHAR(500),
    duration_ms INTEGER,
    added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(spotify_id, track_id)
);

-- 3. Indices
CREATE INDEX IF NOT EXISTS idx_releases_artist_id ON gemfactory.releases(artist_id);
CREATE INDEX IF NOT EXISTS idx_releases_date ON gemfactory.releases(date);
CREATE INDEX IF NOT EXISTS idx_releases_artist_date_track ON gemfactory.releases(artist_id, date, title_track);
CREATE INDEX IF NOT EXISTS idx_artists_name ON gemfactory.artists(name);
CREATE INDEX IF NOT EXISTS idx_artists_gender ON gemfactory.artists(gender);
CREATE INDEX IF NOT EXISTS idx_artists_active ON gemfactory.artists(is_active);
CREATE INDEX IF NOT EXISTS idx_homeworks_user_id ON gemfactory.homeworks(user_id);
CREATE INDEX IF NOT EXISTS idx_homeworks_track_id ON gemfactory.homeworks(track_id);
CREATE INDEX IF NOT EXISTS idx_homeworks_spotify_id ON gemfactory.homeworks(spotify_id);
CREATE INDEX IF NOT EXISTS idx_homeworks_completed ON gemfactory.homeworks(is_completed);
CREATE INDEX IF NOT EXISTS idx_homeworks_user_completed ON gemfactory.homeworks(user_id, is_completed);
CREATE INDEX IF NOT EXISTS idx_playlist_tracks_spotify_id ON gemfactory.playlist_tracks(spotify_id);
CREATE INDEX IF NOT EXISTS idx_playlist_tracks_track_id ON gemfactory.playlist_tracks(track_id);
CREATE INDEX IF NOT EXISTS idx_playlist_tracks_artist ON gemfactory.playlist_tracks(artist);
CREATE INDEX IF NOT EXISTS idx_releases_source_url ON gemfactory.releases(source_url);

-- 4. Initial Seeding
INSERT INTO gemfactory.config (key, value, description) VALUES
('RATE_LIMIT_REQUESTS', '10', 'Rate limit requests per window'),
('RATE_LIMIT_WINDOW', '60', 'Rate limit window in seconds'),
('SCRAPER_DELAY', '1', 'Scraper delay between requests in seconds'),
('SCRAPER_TIMEOUT', '30', 'Scraper request timeout in seconds'),
('LOG_LEVEL', 'info', 'Logging level (debug, info, warn, error, fatal)'),
('BOT_TOKEN', '', 'Telegram bot token'),
('ADMIN_USERNAME', '', 'Administrator username'),
('SPOTIFY_CLIENT_ID', '', 'Spotify client ID'),
('SPOTIFY_CLIENT_SECRET', '', 'Spotify client secret'),
('PLAYLIST_URL', '', 'Spotify playlist URL'),
('DB_DSN', '', 'Database connection string'),
('HEALTH_PORT', '8080', 'Health check port'),
('TIMEZONE', 'Europe/Moscow', 'Application timezone'),
('APP_DATA_DIR', './data', 'Application data directory'),
('RELEASE_CHECK_INTERVAL', '24h', 'Interval between release checks (e.g., 1h, 24h, 30m)')
ON CONFLICT (key) DO NOTHING;

-- Tasks seeding removed

-- Artist Seeding (Sample/Primary list)
INSERT INTO gemfactory.artists (name, gender, is_active) VALUES
('ARTMS', 'female', true), ('Apink', 'female', true), ('AtHeart', 'female', true), ('BABYMONSTER', 'female', true), ('BADVILLAIN', 'female', true),
('BEWAVE', 'female', true), ('BIBI', 'female', true), ('BLACKPINK', 'female', true), ('BURVEY', 'female', true), ('Baby DONT Cry', 'female', true),
('Billlie', 'female', true), ('CHUU', 'female', true), ('CLASS:y', 'female', true), ('CRAZANGEL', 'female', true), ('Candy Shop', 'female', true),
('CHUNG HA', 'female', true), ('DreamNote', 'female', true), ('FIFTY FIFTY', 'female', true), ('GFRIEND', 'female', true), ('Gyubin', 'female', true),
('H1-KEY', 'female', true), ('HITGS', 'female', true), ('Hearts2Hearts', 'female', true), ('HyunA', 'female', true), ('ICHILLIN''', 'female', true),
('ILLIT', 'female', true), ('IRENE', 'female', true), ('IRENE & SEULGI', 'female', true), ('ITZY', 'female', true), ('IU', 'female', true),
('IVE', 'female', true), ('JENNIE', 'female', true), ('JEON SOMI', 'female', true), ('KIIRAS', 'female', true), ('KISS OF LIFE', 'female', true),
('KWON EUNBI', 'female', true), ('Kandis', 'female', true), ('Kep1er', 'female', true), ('KiiiKiii', 'female', true), ('Kim Lip', 'female', true),
('LE SSERAFIM', 'female', true), ('LIGHTSUM', 'female', true), ('LISA', 'female', true), ('MEOVV', 'female', true), ('MISAMO', 'female', true),
('MIYEON', 'female', true), ('MRCH', 'female', true), ('Meowmiro', 'female', true), ('Minnie', 'female', true), ('Moon Byul', 'female', true),
('MΛDEIN', 'female', true), ('NANA', 'female', true), ('NMIXX', 'female', true), ('NiziU', 'female', true), ('ODD YOUTH', 'female', true),
('OH MY GIRL', 'female', true), ('Olivia Marsh', 'female', true), ('PRIMROSE', 'female', true), ('PURPLE KISS', 'female', true), ('QWER', 'female', true),
('RESCENE', 'female', true), ('ROSÉ', 'female', true), ('Red Velvet', 'female', true), ('Rolling Quartz', 'female', true), ('SAY MY NAME', 'female', true),
('SEULGI', 'female', true), ('SOORIN', 'female', true), ('SPIA', 'female', true), ('STAYC', 'female', true), ('SUMMER CAKE', 'female', true),
('SUNMI', 'female', true), ('Solar', 'female', true), ('TAEYEON', 'female', true), ('TWICE', 'female', true), ('UAU', 'female', true),
('UDTT', 'female', true), ('UNIS', 'female', true), ('VIVIZ', 'female', true), ('VVS', 'female', true), ('VVUP', 'female', true),
('WENDY', 'female', true), ('WJSN', 'female', true), ('WOOAH', 'female', true), ('XG', 'female', true), ('YEJI', 'female', true),
('YENA', 'female', true), ('YOU DAYEON', 'female', true), ('YOUNG POSSE', 'female', true), ('YUJU', 'female', true), ('YUQI', 'female', true),
('Yves', 'female', true), ('ablume', 'female', true), ('aespa', 'female', true), ('dodree (도드리)', 'female', true), ('fromis_9', 'female', true), ('hanhee', 'female', true),
('i-dle', 'female', true), ('ifeye', 'female', true), ('izna', 'female', true), ('rescence', 'female', true), ('tripleS', 'female', true),
('tripleS ∞!', 'female', true), ('&TEAM', 'male', true), ('82MAJOR', 'male', true), ('8TURN', 'male', true), ('AB6IX', 'male', true),
('ALL(H)OURS', 'male', true), ('ALLDAY PROJECT', 'male', true), ('AMPERS&ONE', 'male', true), ('ATEEZ', 'male', true), ('BAND LUCY', 'male', true),
('BOYNEXTDOOR', 'male', true), ('BamBam', 'male', true), ('CHEN', 'male', true), ('CORTIS', 'male', true), ('ENHYPEN', 'male', true),
('Forestella', 'male', true), ('GOT7', 'male', true), ('HIGHLIGHT', 'male', true), ('IDID', 'male', true), ('KARD', 'male', true),
('KAVE', 'male', true), ('KEY', 'male', true), ('Kai', 'male', true), ('KickFlip', 'male', true), ('NCT DREAM', 'male', true),
('NEXZ', 'male', true), ('NouerA', 'male', true), ('P1Harmony', 'male', true), ('RIIZE', 'male', true), ('Stray Kids', 'male', true),
('TAEMIN', 'male', true), ('TAEYONG', 'male', true), ('TEN', 'male', true), ('THE BOYZ', 'male', true), ('TXT', 'male', true),
('WHIB', 'male', true), ('WayV', 'male', true), ('Xdinary Heroes', 'male', true), ('ZEROBASEONE', 'male', true), ('idntt', 'male', true),
('male &TEAM', 'male', true), ('xikers', 'male', true)
ON CONFLICT (name) DO NOTHING;
