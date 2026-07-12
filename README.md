# 🍿 Anime Sentinel 
An independent, offline-first background service and REST API built in Go. It operates on local home lab hardware to aggregate, normalize, and track media consumption for multiple isolated user profiles.

This repository serves as the foundational blueprint for **The Sentinel Suite**—a tri-repository microservice architecture designed to share a single data backend via infrastructure configuration rather than tightly coupled code.

---

## 🏗️ The Sentinel Suite Architecture
```
┌───────────────────────────────────────────────────┐
│            Media Sentinel (Future UI)             │
└──────────────────────────┬────────────────────────┘
                           │ (Aggregates API Feeds)
  ┌────────────────────────┼────────────────────────┐
  ▼                        ▼                        ▼
┌────────────────┐ ┌───────────────┐ ┌────────────────┐
│ Anime Sentinel │ │  TV Sentinel  │ │ Movie Sentinel │
│   (Repo #1)    │ │   (Repo #2)   │ │   (Repo #3)    │
└───────┬────────┘ └──────┬────────┘ └───────┬────────┘
        │                 │                  │
   └────────────────────┐ │ ┌──────────────────────┘
					    ▼ ▼ ▼
		┌───────────────────────────────────┐
		│      shared-sentinel-data/        │
		│        sentinel_suite.db          │
		└───────────────────────────────────┘
```

## Concurrent Background Processing Flow & Stampede Mitigation
```
[Gateway Intake]
  │  POST /api/v1/ingest (Sub-2ms validation)
  ▼
[Buffered Channel]
  │  Capacity: 10,000 tasks
  ▼
[Worker Routine Pool]
  │  4 Parallel Goroutines draining the channel
  ▼
[Step 1: Staging Log]
  │  Commit raw entry directly to 'ingest_staging_history'
  ▼
[Step 2: Read-Through Cache Lookup]
  │  Check local 'media_catalog' table
  │
  ├───► (Cache HIT) ──────────────────────────────┐
  │                                               │
  └───► (Cache MISS)                              │
        ▼                                         │
  [Shared Token Gate]                             │
        │  Block against 700ms internal Ticker    │
        ▼                                         │
  [Step 3: Lock-Free Double Check]                │
        │  Query local DB catalog one more time   │
        │                                         │
        ├───► (Stampede Mitigated: HIT) ──────────┤
        │                                         │
        └───► (True Miss: Hit Network)            │
              ▼                                   │
        [AniList GraphQL API]                     │
              │  Outbound query dispatch          │
              ▼                                   │
        [Cache Populate Layer]                    │
              │  Insert new row into catalog      │
              ▼                                   ▼
        [Update Progress State Engine] ───────────────► [Watch Progress Ledger]
```

## 📊 Joint-Viewing Intelligence & Recommendation Pipeline
```
[Gateway Intake]
│ GET /api/v1/analytics/shared?user_a=1&user_b=2
▼
[Profile Overlap Analysis]
│ Fetch local UserEngagement profiles out-of-band
▼
[Mathematical Affinity Intersection]
│ Isolate mutual taste anchors (Completion Depth >= 80%)
│ Compute Compatibility Score: (Mutual Anchors / Total Unique Titles) * 100
▼
[External Relational Seed Mapping]
│ Resolve local MediaID to string ExternalID via cache layer
│ Select top 3 shared anchors maximum as graph query hooks
▼
[Cooperative Token Gate]
│ Synchronize lookups through shared 700ms client Ticker
▼
[AniList Relational Edge Query]
│ Fetch community recommendations (RATING_DESC, max 10 per seed)
▼
[Double-Cache Filtering & Deduplication]
│ Verify recommended titles against both English and Romaji variants
│ Drop title if either user has a matching record in local watch logs
▼
[Clean JSON Response Stream]
└─► Return Compatibility Affinity % alongside curated recommendation nodes
```

## Core Philosophy & Constraints
1. **Zero External Runtime Dependencies:** Built strictly using the Go standard library (`net/http`, `slog`, `regexp`, `database/sql`, `sync/atomic`) to minimize container footprint and maximize execution velocity.
2. **Implicit Engagement Tracking:** Eliminates explicit user rating matrices. Taste anchors and enjoyment metrics are calculated programmatically through completion depth **Engagement Score $\ge$ 80%**.
3. **Decoupled Data Infrastructure:** Configuration parameters point to a central, un-tracked SQLite file using Write-Ahead Logging (`WAL` mode) to allow multi-process concurrency across the suite.
4. **Structured DevOps Telemetry:** Built with consistent, scannable log tokens (`[INIT]`, `[SECURE]`, `[IDLE]`, `[REALTIME]`, `[SERVER]`, `[OK]`, `[ERROR]`, `[SHUTDOWN]`) for clean, production-grade terminal visibility.
5. **Graceful Pipeline Teardown:** Listens explicitly for OS lifecycle interrupts (`SIGINT`, `SIGTERM`). On capture, the API gateway locks down instantly, tickers drop safely, and SQLite connection pools execute a full final checkpoint—collapsing active `-wal` and `-shm` disk fragments back down into a single consolidated database file.

## 🛠️ Tech Stack & Runtime
- **Language Runtime:** Go 1.24+ (Native structured logging, atomic concurrency primitives, and enhanced HTTP routing)
- **Database Engine:** SQLite 3 via `github.com/mattn/go-sqlite3`
- **Metadata Authority:** AniList GraphQL API (Enforced via a thread-safe 700ms token ticker coordinating cooperative worker pacing below 90 reqs/min)
- **Deployment Target:** Docker Multi-stage scratch container

## 🗺️ Project Roadmap
### Phase 1: Core Scaffolding & Parsing (Completed)
- [x] Establish isolated repository workspace and module layout.
- [x] Construct standard library network health router loop on custom port options.
- [x] Author regex parser engine to strip subtitles and extract implicit media format states.
- [x] Design externalized `schema.sql` automation script and setup concurrent SQLite WAL connection layer.

### Phase 2: Ingestion & Concurrency Pipeline (Completed)
- [x] Model core data access objects (DAOs) inside `pkg/models` to mirror database constraints.
- [x] Build a high-volume, non-blocking asynchronous ingestion route utilizing a 10,000-capacity buffered channel to absorb historical user data under 2ms.
- [x] Construct a concurrent background worker routine pool (4 workers) to process ingested payloads out-of-band using method receiver patterns.
- [x] Implement structured logging telemetry (`slog`) throughout the boot sequence and handler loops to ensure machine-readable application metrics.

### Phase 3: Processing Pipeline, Metadata Aggregation, & Shutdown (Completed)
- [x] Connect background workers to `parser.NormalizeWatchEntry` for live title transformation out-of-band.
- [x] Implement background worker storage logic to log incoming payloads into the `ingest_staging_history` tracking tables.
- [x] Construct a throttled GraphQL client to execute external AniList metadata queries without breaking remote rate limits.
- [x] Design a high-performance **Read-Through Cache Layer** to intercept redundant network calls and shield remote API quotas.
- [x] Integrate a lock-free **Cooperative Double-Check Mechanism** post-token retrieval to dynamically eliminate cache stampedes over concurrent bursts.
- [x] Implement an **Atomic Idle State Monitor** utilizing `sync/atomic` CAS switches to signal real-time queue depletion via ANSI terminal visuals.
- [x] Introduce an **OS Signal Interceptor** to safely close background processes and enforce explicit SQLite WAL log file collapse during server shutdowns.

### Phase 4: Taste Analytical Intelligence (Completed)
- [x] Code the taste profile engine and calculate completion metrics based on the 80% engagement rule.
- [x] Expose an analytical intelligence endpoint (`GET /api/v1/analytics/taste`) to resolve user preference anchors.
- [x] Implement Joint-Viewing Delta calculation endpoints (`GET /api/v1/analytics/shared`) to cross-reference profiles, compute mutual affinity, and fetch curated external recommendations.