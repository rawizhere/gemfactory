-- GemFactory Consolidated Initialization Schema
CREATE SCHEMA IF NOT EXISTS gemfactory;
SET search_path TO gemfactory, public;

-- Drop legacy/removed tables if they exist
DROP TABLE IF EXISTS gemfactory.playlist_tracks CASCADE;
DROP TABLE IF EXISTS gemfactory.homeworks CASCADE;
DROP TABLE IF EXISTS gemfactory.tasks CASCADE;

-- 1. Artists Table
CREATE TABLE IF NOT EXISTS gemfactory.artists (
    artist_id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    gender VARCHAR(10) NOT NULL CHECK (gender IN ('female', 'male', 'mixed')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 2. Releases Table
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

-- 3. Config Table
CREATE TABLE IF NOT EXISTS gemfactory.config (
    id SERIAL PRIMARY KEY,
    key VARCHAR(255) NOT NULL UNIQUE,
    value TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Delete deprecated config keys
DELETE FROM gemfactory.config WHERE key IN ('SPOTIFY_CLIENT_ID', 'SPOTIFY_CLIENT_SECRET', 'PLAYLIST_URL', 'HOMEWORK_RESET_TIME');

-- Clean up legacy duplicate releases (keeping the one with links or highest ID)
DELETE FROM gemfactory.releases r1
WHERE EXISTS (
    SELECT 1 FROM gemfactory.releases r2
    WHERE r2.artist_id = r1.artist_id
      AND r2.date = r1.date
      AND (
          (COALESCE(r2.mv, '') != '' AND COALESCE(r1.mv, '') = '')
          OR (COALESCE(r2.spotify, '') != '' AND COALESCE(r1.spotify, '') = '')
          OR (COALESCE(r2.mv, '') = COALESCE(r1.mv, '') AND COALESCE(r2.spotify, '') = COALESCE(r1.spotify, '') AND r2.release_id > r1.release_id)
      )
);

-- 4. Indices
CREATE UNIQUE INDEX IF NOT EXISTS idx_releases_artist_date_source ON gemfactory.releases(artist_id, date, source_url);
CREATE INDEX IF NOT EXISTS idx_releases_artist_id ON gemfactory.releases(artist_id);
CREATE INDEX IF NOT EXISTS idx_releases_date ON gemfactory.releases(date);
CREATE INDEX IF NOT EXISTS idx_releases_artist_date_track ON gemfactory.releases(artist_id, date, title_track);
CREATE INDEX IF NOT EXISTS idx_releases_source_url ON gemfactory.releases(source_url);
CREATE INDEX IF NOT EXISTS idx_artists_name ON gemfactory.artists(name);
CREATE INDEX IF NOT EXISTS idx_artists_gender ON gemfactory.artists(gender);
CREATE INDEX IF NOT EXISTS idx_artists_active ON gemfactory.artists(is_active);

-- 5. Initial Seeding
INSERT INTO gemfactory.config (key, value, description) VALUES
('RATE_LIMIT_REQUESTS', '10', 'Rate limit requests per window'),
('RATE_LIMIT_WINDOW', '60', 'Rate limit window in seconds'),
('SCRAPER_DELAY', '2s', 'Scraper delay between requests'),
('LOG_LEVEL', 'info', 'Logging level (debug, info, warn, error, fatal)'),
('BOT_TOKEN', '', 'Telegram bot token'),
('ADMIN_USERNAME', '', 'Administrator username'),
('DB_DSN', '', 'Database connection string'),
('HEALTH_PORT', '8080', 'Health check port'),
('TIMEZONE', 'Europe/Moscow', 'Application timezone'),
('APP_DATA_DIR', './data', 'Application data directory'),
('RELEASE_CHECK_INTERVAL', '24h', 'Interval between release checks')
ON CONFLICT (key) DO NOTHING;

-- 6. Artists Seeding
INSERT INTO gemfactory.artists (name, gender, is_active) VALUES
('ARTMS', 'female', true), ('Apink', 'female', true), ('AtHeart', 'female', true), ('BABYMONSTER', 'female', true), ('BADVILLAIN', 'female', true),
('BEWAVE', 'female', true), ('BIBI', 'female', true), ('BLACKPINK', 'female', true), ('BURVEY', 'female', true), ('Baby DONT Cry', 'female', true),
('Billlie', 'female', true), ('CHUNG HA', 'female', true), ('CHUU', 'female', true), ('CLASS:y', 'female', true), ('CRAZANGEL', 'female', true),
('Candy Shop', 'female', true), ('DreamNote', 'female', true), ('FIFTY FIFTY', 'female', true), ('GFRIEND', 'female', true), ('Gyubin', 'female', true),
('H1-KEY', 'female', true), ('HITGS', 'female', true), ('Hearts2Hearts', 'female', true), ('HyunA', 'female', true), ('ICHILLIN''', 'female', true),
('ILLIT', 'female', true), ('IRENE', 'female', true), ('IRENE & SEULGI', 'female', true), ('ITZY', 'female', true), ('IU', 'female', true),
('IVE', 'female', true), ('JENNIE', 'female', true), ('JEON SOMI', 'female', true), ('KIIRAS', 'female', true), ('KISS OF LIFE', 'female', true),
('KWON EUNBI', 'female', true), ('Kandis', 'female', true), ('Kep1er', 'female', true), ('KiiiKiii', 'female', true), ('Kim Lip', 'female', true),
('LE SSERAFIM', 'female', true), ('LIGHTSUM', 'female', true), ('LISA', 'female', true), ('LOLA', 'female', true),
('MADEIN', 'female', true), ('MEOVV', 'female', true), ('MISAMO', 'female', true), ('MIYEON', 'female', true), ('MRCH', 'female', true),
('Meowmiro', 'female', true), ('Minnie', 'female', true), ('Moon Byul', 'female', true), ('MΛDEIN', 'female', true), ('NANA', 'female', true),
('NMIXX', 'female', true), ('NiziU', 'female', true), ('ODD YOUTH', 'female', true), ('OH MY GIRL', 'female', true), ('Olivia Marsh', 'female', true),
('PRIMROSE', 'female', true), ('PURPLE KISS', 'female', true), ('QWER', 'female', true), ('RESCENE', 'female', true), ('ROSÉ', 'female', true),
('Red Velvet', 'female', true), ('Rolling Quartz', 'female', true), ('SAY MY NAME', 'female', true), ('SEULGI', 'female', true), ('SOORIN', 'female', true),
('SPIA', 'female', true), ('STAYC', 'female', true), ('SUMMER CAKE', 'female', true), ('SUNMI', 'female', true), ('Solar', 'female', true),
('TAEYEON', 'female', true), ('TWICE', 'female', true), ('UAU', 'female', true), ('UDTT', 'female', true), ('UNIS', 'female', true),
('VIVIZ', 'female', true), ('VVS', 'female', true), ('VVUP', 'female', true), ('WENDY', 'female', true), ('WJSN', 'female', true),
('WOOAH', 'female', true), ('XG', 'female', true), ('YEJI', 'female', true), ('YENA', 'female', true), ('YOU DAYEON', 'female', true),
('YOUNG POSSE', 'female', true), ('YUJU', 'female', true), ('YUQI', 'female', true), ('Yves', 'female', true), ('ablume', 'female', true),
('aespa', 'female', true), ('dodree', 'female', true), ('fromis_9', 'female', true), ('hanhee', 'female', true), ('i-dle', 'female', true),
('ifeye', 'female', true), ('izna', 'female', true), ('rescence', 'female', true), ('tripleS', 'female', true),
('&TEAM', 'male', true), ('82MAJOR', 'male', true), ('8TURN', 'male', true), ('AB6IX', 'male', true),
('ALL(H)OURS', 'male', true), ('ALLDAY PROJECT', 'male', true), ('ALPHA DRIVE ONE', 'male', true), ('AMPERS&ONE', 'male', true), ('ATEEZ', 'male', true),
('AxMxP', 'male', true), ('BAND LUCY', 'male', true), ('BOYNEXTDOOR', 'male', true), ('BamBam', 'male', true), ('CHEN', 'male', true),
('CORTIS', 'male', true), ('ENHYPEN', 'male', true), ('Forestella', 'male', true), ('GOT7', 'male', true), ('HIGHLIGHT', 'male', true),
('IDID', 'male', true), ('JOOHONEY', 'male', true), ('KARD', 'male', true), ('KAVE', 'male', true), ('KEY', 'male', true),
('Kai', 'male', true), ('KickFlip', 'male', true), ('LA POEM', 'male', true), ('NCT DREAM', 'male', true), ('NEXZ', 'male', true),
('NouerA', 'male', true), ('P1Harmony', 'male', true), ('RIIZE', 'male', true), ('Stray Kids', 'male', true), ('TAEMIN', 'male', true),
('TAEYONG', 'male', true), ('TEN', 'male', true), ('THE BOYZ', 'male', true), ('TXT', 'male', true), ('WAKER', 'male', true),
('WHIB', 'male', true), ('WayV', 'male', true), ('Xdinary Heroes', 'male', true), ('ZEROBASEONE', 'male', true), 
('idntt', 'male', true), ('xikers', 'male', true)
ON CONFLICT (name) DO NOTHING;
