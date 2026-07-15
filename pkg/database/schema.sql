-- ====================================================================
-- THE SENTINEL SUITE: UNIFIED ANIME EXTENSION SCHEMA (V2.1 - SPLIT CACHE)
-- ====================================================================

-- --------------------------------------------------------------------
--          -- LOCAL USER IDENTITIES (SHARED SUITE ANCHOR) --
-- Included here via IF NOT EXISTS to guarantee cross-repo consistency.
-- Stores system account profiles responsible for active media tracking.
-- --------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- --------------------------------------------------------------------
--       -- NORMALIZED ANIME CATALOG (AUTONOMOUS CACHE LAYER) --
-- Acts as a read-through localized lookup layer to shield AniList API quotas.
-- Core metadata table tracks titles, media format, and status fields.
-- --------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS anime_catalog (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    external_id TEXT NOT NULL UNIQUE,        -- AniList serial string identifier
    cache_key TEXT NOT NULL UNIQUE,          -- Normalized lower-case base search key
    title_romaji TEXT NOT NULL,              -- Official romaji presentation title
    title_english TEXT,                      -- Official english presentation title
    format TEXT CHECK(format IN ('TV', 'MOVIE', 'OVA', 'SPECIAL', 'ONA', 'TV_SHORT')),
    status TEXT CHECK(status IN ('FINISHED', 'RELEASING', 'NOT_YET_RELEASED')),
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- --------------------------------------------------------------------
--        -- CATALOG EPISODIC DEPTHS (EPISODIC CACHE LAYER) --
-- Maps the expected total episode counts to individual catalog items.
-- Decoupling this enables future-proof tracking of multi-arc metadata.
-- --------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS anime_catalog_depths (
    anime_id INTEGER PRIMARY KEY,
    total_episodes_count INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(anime_id) REFERENCES anime_catalog(id) ON DELETE CASCADE
);

-- --------------------------------------------------------------------
--    -- CATALOG METADATA WEIGHTS FOR AUTOMATED TASTE PROFILES --
-- Maps relational category classifiers to physical anime catalog items.
-- Cascade deletions ensure that orphaned tags drop cleanly if a title is removed.
-- --------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS anime_catalog_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    anime_id INTEGER NOT NULL,
    type TEXT CHECK(type IN ('GENRE', 'TAG')),
    name TEXT NOT NULL,
    FOREIGN KEY(anime_id) REFERENCES anime_catalog(id) ON DELETE CASCADE
);

-- --------------------------------------------------------------------
--              -- ISOLATED ANIME PROGRESS LEDGERS --
-- Evaluates real-time milestone checkboxes, tracking absolute episode progress decimal scales
-- and current affinity sentiment values bound to individual unique user profiles.
-- --------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS anime_watch_progress (
    user_id INTEGER,
    anime_id INTEGER,
    current_episode_progress REAL DEFAULT 0.0,
    sentiment INTEGER NOT NULL CHECK(sentiment IN (-1, 0, 1)) DEFAULT 0,
    last_watched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, anime_id),
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY(anime_id) REFERENCES anime_catalog(id) ON DELETE CASCADE
);

-- --------------------------------------------------------------------
--    -- HIGH-VOLUME INGESTION STAGING TABLE (ISOLATED ANIME SINK) --
-- Provides a dedicated staging sandbox to prevent cross-service lock contention.
-- Relaxes upstream relational constraints to maximize non-blocking input rates.
-- --------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS anime_ingest_staging_history (
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

-- --------------------------------------------------------------------
--              -- HIGH-PERFORMANCE QUERY OPTIMIZATIONS --
-- Explicitly constructed indexes to accelerate fast key scans, user profile updates,
-- and background processing engine task queries.
-- --------------------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_anime_catalog_lookup ON anime_catalog(cache_key);
CREATE INDEX IF NOT EXISTS idx_anime_progress_user ON anime_watch_progress(user_id);
CREATE INDEX IF NOT EXISTS idx_anime_ingest_status ON anime_ingest_staging_history(processed_status);