-- ====================================================================
-- THE SENTINEL SUITE: UNIFIED CORE SCHEMA (V2 - CRUNCHY-STANDARDIZED)
-- ====================================================================

-- 1. Local User Identities
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 2. Normalized Media Catalog (Autonomous Cache Layer)
CREATE TABLE IF NOT EXISTS media_catalog (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    external_id TEXT NOT NULL UNIQUE,   -- Maps directly to AniList/External API IDs
    cache_key TEXT UNIQUE,
    title_romaji TEXT NOT NULL,
    title_english TEXT,
    format TEXT CHECK(format IN ('TV', 'MOVIE', 'OVA', 'SPECIAL', 'ONA', 'TV_SHORT')),
    status TEXT CHECK(status IN ('FINISHED', 'RELEASING', 'NOT_YET_RELEASED')),
    total_episodes_count INTEGER DEFAULT 1,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 3. Catalog Metadata Weights for Automated Taste Profiles
CREATE TABLE IF NOT EXISTS catalog_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id INTEGER NOT NULL,
    type TEXT CHECK(type IN ('GENRE', 'TAG')),
    name TEXT NOT NULL,
    FOREIGN KEY(media_id) REFERENCES media_catalog(id) ON DELETE CASCADE
);

-- 4. Isolated User Progress Ledgers
CREATE TABLE IF NOT EXISTS watch_progress (
    user_id INTEGER,
    media_id INTEGER,
    current_episode_progress REAL DEFAULT 0.0,
    sentiment INTEGER NOT NULL CHECK(sentiment IN (-1, 0, 1)) DEFAULT 0,
    last_watched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, media_id),
    FOREIGN KEY(user_id) REFERENCES users(id),
    FOREIGN KEY(media_id) REFERENCES media_catalog(id) ON DELETE CASCADE
);

-- 5. High-Volume Ingestion Staging Table
CREATE TABLE IF NOT EXISTS ingest_staging_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    series_id TEXT NOT NULL,
    series_title TEXT NOT NULL,
    season_number INTEGER NOT NULL,
    episode_number REAL NOT NULL,
    episode_title TEXT,
    episode_id TEXT,
    watched_at DATETIME NOT NULL,
    fully_watched BOOLEAN NOT NULL CHECK (fully_watched IN (0, 1)),
    sentiment INTEGER NOT NULL CHECK (sentiment IN (-1, 0, 1)) DEFAULT 0,
    processed_status TEXT CHECK(processed_status IN ('PENDING', 'PROCESSED', 'FAILED')) DEFAULT 'PENDING',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 6. High-Performance Query Optimizations
CREATE INDEX IF NOT EXISTS idx_catalog_lookup ON media_catalog(external_id);
CREATE INDEX IF NOT EXISTS idx_progress_user ON watch_progress(user_id);
CREATE INDEX IF NOT EXISTS idx_ingest_status ON ingest_staging_history(processed_status);