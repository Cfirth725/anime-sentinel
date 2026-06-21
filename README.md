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

### Concurrent Background Processing Flow (Phase 3)
```
       [Public HTTP Gateway] (POST /api/v1/ingest)
                 │
                 ▼  (Validates payloads rapidly in < 2ms)
         [Buffered Channel] (Capacity: 10,000)
                 │
       ┌─────────┴─────────┬─────────────────┐
       ▼                   ▼                 ▼
  [Worker 1]          [Worker 2]        [Worker N...] (Autonomous Goroutines)
       │                   │                 │
       ├───────────────────┴─────────────────┤
       ▼                                     ▼
[Staging Storage]                     [Metadata Client]
(SQLite WAL Engine)                   (700ms time.Ticker Throttle)
                                             │
                                             ▼
                                      [AniList GraphQL API]
```

## Core Philosophy & Constraints
1. **Zero External Runtime Dependencies:** Built strictly using the Go standard library (`net/http`, `slog`, `regexp`, `database/sql`) to minimize container footprint and maximize execution velocity.
2. **Implicit Engagement Tracking:** Eliminates explicit user rating matrices. Taste anchors and enjoyment metrics are calculated programmatically through completion depth **Engagement Score $\ge$ 80%**.
3. **Decoupled Data Infrastructure:** Configuration parameters point to a central, un-tracked SQLite file using Write-Ahead Logging (`WAL` mode) to allow multi-process concurrency across the suite.

## 🛠️ Tech Stack & Runtime
- **Language Runtime:** Go 1.24+ (Native structured logging and enhanced HTTP routing patterns)
- **Database Engine:** SQLite 3 via `github.com/mattn/go-sqlite3`
- **Metadata Authority:** AniList GraphQL API (Enforced via safe, internal `time.Ticker` rate limiter throttled to a max of ~85 requests/minute)
- **Deployment Target:** Docker Multi-stage scratch container

## 📂 System Topology
```
anime-sentinel/
├── cmd/
│   └── server/
│       └── main.go       # HTTP Router & dependency injection entry point
├── pkg/
│   ├── database/
│   │   ├── db.go         # SQLite engine setup & WAL configuration
│   │   └── schema.sql    # Relational DDL tables and indexing strategies
│   ├── ingest/
│   │   └── engine.go     # Asynchronous channel buffer & concurrent worker pool
│   ├── metadata/
│   │   └── client.go     # Throttled GraphQL HTTP client with time.Ticker control
│   ├── models/
│   │   ├── anime.go      # Relational database struct mappings
│   │   └── anilist.go    # Upstream GraphQL query and variable contract models
│   └── parser/
│       └── regex.go      # Title text cleaning & normalization engine
├── config.json           # Local execution configuration 
└── README.md             # Project roadmap & technical specification
```

## 🗺️ Project Roadmap
### Phase 1: Core Scaffolding & Parsing (Completed)
- [x] Establish isolated repository workspace and module layout.
- [x] Construct standard library network health router loop on custom port `8092`.
- [x] Author regex parser engine to strip subtitles and extract implicit media format states.
- [x] Design externalized `schema.sql` automation script and setup concurrent SQLite WAL connection layer.

### Phase 2: Ingestion & Concurrency Pipeline (Completed)
- [x] Model core data access objects (DAOs) inside `pkg/models` to mirror database constraints.
- [x] Build a high-volume, non-blocking asynchronous ingestion route utilizing a 10,000-capacity buffered channel to absorb historical user data under 2ms.
- [x] Construct a concurrent background worker routine pool (4 workers) to process ingested payloads out-of-band using method receiver patterns.
- [x] Implement structured logging telemetry (`slog`) throughout the boot sequence and handler loops to ensure machine-readable application metrics.

### Phase 3: The Processing Pipeline & Metadata Aggregation (Completed)
- [x] Connect background workers to `parser.NormalizeWatchEntry` for live title transformation out-of-band.
- [x] Implement background worker storage logic to log incoming payloads into the `ingest_staging_history` tracking tables.
- [x] Construct a throttled GraphQL client using an internal ticker loop to execute external AniList metadata queries without breaking remote rate limits (90 requests/min maximum).

### Phase 4: Co-Viewing Intelligence Engine (Upcoming)
- [ ] Code the taste profile calculator using the mathematical engagement index.
- [ ] Implement the Joint-Viewing Delta calculation endpoint (`GET /api/v1/recommendations/shared`) to discover mutual watch interests while handling release anomalies automatically.
