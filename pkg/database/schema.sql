-- ====================================================================
-- THE SENTINEL SUITE: UNIFIED CORE SCHEMA
-- ====================================================================

-- Local User Identities
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Normalized Media Catalog (Autonomous Cache Layer)
CREATE TABLE IF NOT EXISTS media_catalog (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    external_id TEXT NOT NULL UNIQUE,   -- Maps directly to AniList/External API IDs
    search_query TEXT UNIQUE,           -- Caches the original clean scraper search term
    title_romaji TEXT NOT NULL,
    title_english TEXT,
    format TEXT CHECK(format IN ('TV', 'MOVIE', 'OVA', 'SPECIAL', 'ONA', 'TV_SHORT')),
    status TEXT CHECK(status IN ('FINISHED', 'RELEASING', 'NOT_YET_RELEASED')),
    total_episodes_count INTEGER DEFAULT 1,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Catalog Metadata Weights for Automated Taste Profiles
CREATE TABLE IF NOT EXISTS catalog_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id INTEGER NOT NULL,
    type TEXT CHECK(type IN ('GENRE', 'TAG')),
    name TEXT NOT NULL,
    FOREIGN KEY(media_id) REFERENCES media_catalog(id) ON DELETE CASCADE
);

-- Isolated User Progress Ledgers
CREATE TABLE IF NOT EXISTS watch_progress (
    user_id INTEGER,
    media_id INTEGER,
    current_episode_progress INTEGER DEFAULT 0,
    sentiment INTEGER NOT NULL CHECK(sentiment IN (-1, 0, 1)) DEFAULT 0,
    last_watched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, media_id),
    FOREIGN KEY(user_id) REFERENCES users(id),
    FOREIGN KEY(media_id) REFERENCES media_catalog(id) ON DELETE CASCADE
);

-- High-Volume Ingestion Staging Table (Tracks normalized entries before external API sync)
CREATE TABLE IF NOT EXISTS ingest_staging_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    normalized_title TEXT NOT NULL,
    episode_number INTEGER NOT NULL,
    is_movie INTEGER NOT NULL CHECK (is_movie IN (0, 1)),
    sentiment INTEGER NOT NULL CHECK (sentiment IN (-1, 0, 1)) DEFAULT 0,
    processed_status TEXT CHECK(processed_status IN ('PENDING', 'PROCESSED', 'FAILED')) DEFAULT 'PENDING',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- High-Performance Query Optimizations
CREATE INDEX IF NOT EXISTS idx_catalog_lookup ON media_catalog(external_id);
CREATE INDEX IF NOT EXISTS idx_progress_user ON watch_progress(user_id);
CREATE INDEX IF NOT EXISTS idx_ingest_status ON ingest_staging_history(processed_status);