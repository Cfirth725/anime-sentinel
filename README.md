# 🍿Anime Sentinel 
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

## Core Philosophy & Constraints
1. **Zero External Dependencies:** Built strictly using the Go standard library (`net/http`, `slog`, `regexp`, `database/sql`) to minimize container footprint and maximize execution velocity.
2. **Implicit Engagement Tracking:** Eliminates explicit user rating matrices. Taste anchors and enjoyment metrics are calculated programmatically through completion depth **Engagement Score $\ge$ 80%**.
3. **Decoupled Data Infrastructure:** Configuration parameters point to a central, un-tracked SQLite file using Write-Ahead Logging (`WAL` mode) to allow multi-process concurrency across the suite.

---

## 🛠️ Tech Stack & Runtime
* **Language Runtime:** Go 1.22+ (Enhanced standard routing patterns)
* **Database Engine:** SQLite 3 via `github.com/mattn/go-sqlite3`
* **Metadata Authority:** AniList GraphQL API
* **Deployment Target:** Docker Multi-stage scratch container on the Milford Node

---

## 📂 System Topology
```text
anime-sentinel/
├── cmd/
│   └── server/
│       └── main.go       # HTTP Router & dependency injection entry point
├── pkg/
│   ├── database/
│   │   ├── db.go         # SQLite engine setup & WAL configuration
│   │   └── schema.sql    # Relational DDL tables and indexing strategies
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

### Phase 2: Ingestion & Concurrency Pipeline (Upcoming)
- [ ] Model core data access objects (DAOs) inside `pkg/models`.
- [ ] Build high-volume asynchronous ingestion routes utilizing buffered Go channels (`chan`) to absorb historical user data.
- [ ] Construct a throttled background worker pool to execute API metadata queries without triggering external rate limits (90 requests/min maximum).

### Phase 3: Co-Viewing Intelligence Engine
- [ ] Code the taste profile calculator using the mathematical engagement index.
- [ ] Implement the Joint-Viewing Delta calculation endpoint (`GET /api/v1/recommendations/shared`) to discover mutual watch interests while handling release anomalies automatically.
    
