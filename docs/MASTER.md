# Movie Streamer — Master Document

The single entry point for the whole project. Read this first. It defines what
we're building, the architecture, how the pieces talk, and how the code is laid
out. Details live in linked docs.

## Doc map

- **MASTER.md** (this file) — the system, the architecture, the project structure.
- **[DECISIONS.md](./DECISIONS.md)** — every architecture decision with the *why*.
- **features/** — one doc per feature (flow, functions, failure modes).

---

## 1. What we're building

A production-grade movie streaming platform: users **upload** large video files,
a **transcoding pipeline** converts them into streamable adaptive-bitrate
renditions (HLS), and a **stream service** serves them to players. The point is
to learn production concepts — bounded concurrency, retries, failure handling,
async pipelines, observability — not just to make a demo.

## 2. Architecture style — microservices (pragmatic)

We build **3 independently deployable services** that communicate over a **queue**
(async) and **HTTP** (sync), sharing **object storage** and a **metadata DB**.

Honest framing (this is an interview talking point): full microservices from day
one is often *over-engineering*. So we use a **modular monorepo** — one git repo,
shared libraries, but each service compiles to its **own binary + own container**
and scales independently. We get the microservices *deployment* benefits (isolate
failure, scale the transcode workers separately) without the polyrepo overhead of
juggling many repos early. See [ADR-004](./DECISIONS.md).

### The services

| Service | Responsibility | Scales on |
|---|---|---|
| **upload-service** | Accept uploads (presigned URLs), record metadata, enqueue transcode jobs | request volume |
| **transcode-worker** | Consume jobs from RabbitMQ; split source into keyframe-aligned chunks, transcode chunks in parallel (ffmpeg → HLS), fan-in to playlists; handle retries/failure | job backlog (the heavy one) |
| **stream-service** | Serve HLS manifests + segments to players | viewer volume |

### Shared infrastructure

| Component | Local (now) | Cloud (later) | Purpose |
|---|---|---|---|
| Object storage | MinIO | S3 | store raw + transcoded video |
| Queue | RabbitMQ | SQS | decouple upload from transcode (see [ADR-006](./DECISIONS.md)) |
| Metadata DB | Postgres | RDS | movie state, job state |

### How they communicate

```
   client ──POST /uploads──▶  ┌────────────────┐
          ◀─202 + presign PUT │ upload-service │──tx: movies+jobs+outbox──▶ [ Postgres ]
          ──PUT bytes──┐      └───────┬────────┘                              ▲   │
                       │              │ outbox relay reads ──────────────────────┘   │ state
                       ▼              ▼                                              │
                  [  MinIO  ]    [ RabbitMQ ] ──deliver (1 in-flight)──┐             │
                   raw ▲ │ hls    retry / DLX                          ▼             │
                       │ │                              ┌────────────────────────┐  │
        download raw ──┘ └── upload HLS ────────────────│    transcode-worker    │──┘ flip → ready
                                                        │  ffmpeg → HLS; ACK      │  (N replicas)
                                                        └────────────────────────┘
   player ──GET .m3u8──▶ ┌────────────────┐
                         │ stream-service │ ──reads (redirect / CDN)──▶ [ MinIO ]
                         └────────────────┘
```

- **Sync (HTTP):** client→upload, player→stream. Immediate request/response.
- **Async (broker):** upload→transcode. Upload returns instantly; transcoding
  happens later. This decoupling is *why* a slow/failed transcode never blocks an
  upload — the core resilience idea of the whole project.
- **Reliable enqueue:** the upload-service writes the job **and** an `outbox` row in
  one DB transaction; a relay forwards outbox rows to RabbitMQ. This avoids the
  dual-write problem (job committed but message lost). See
  [ADR-007](./DECISIONS.md). Delivery is **at-least-once**, so workers are idempotent.
- Full end-to-end flow, tables, and failure modes: **[features/02](./features/02-transcode-pipeline.md)**.

## 3. Project structure

**Service-per-folder monorepo, layered inside each service** (controller → service →
repository → model), with cross-cutting code in a `shared/` package. One `go.mod`.
Migrations live in a **separate repo**, not in any service. See [ADR-010](./DECISIONS.md).

```
movie-streamer/
├── shared/                     # imported by every service
│   ├── config/                 # env → typed Config
│   ├── database/               # GORM connection (pool, logging) — NO migrations
│   ├── model/                  # domain structs (Movie, …) mapping to the shared DB
│   ├── storage/                # object storage behind an interface (MinIO/S3)
│   └── queue/                  # broker behind an interface (RabbitMQ/SQS)
├── upload-service/             # a deployable service; owns its layers
│   ├── main.go                 # wiring: config → db → repo → service → controller
│   ├── controller/             # gin HTTP handlers (parse/validate, shape response)
│   ├── service/                # business logic (orchestrates repo + storage)
│   └── repository/             # data access (interface + GORM impl)
├── transcode-worker/           # (later) same shape; split/chunk/assemble handlers
├── stream-service/             # (later)
├── outbox-relay/               # (later) forwards outbox rows → RabbitMQ (ADR-007)
├── deploy/                     # docker-compose (Postgres + MinIO + RabbitMQ), Dockerfiles
├── docs/                       # MASTER, DECISIONS, features, LEARNING
├── go.mod
└── Makefile
```

**Layering rule:** dependencies point one way — `controller → service → repository`,
and every layer may use `model`. A service depends on its repository through an
**interface** (constructor injection), so business logic is unit-testable with a fake
repo. Schema is owned by the separate **migrations repo**; GORM models only *map* to
those tables (no `AutoMigrate`).

Why `shared/storage` and `shared/queue` are **interfaces**: business logic depends on an
interface, not on MinIO or RabbitMQ directly. Swapping to S3/SQS later is a new
implementation, not a rewrite. This is [ADR-002](./DECISIONS.md) made concrete.

## 4. Milestone roadmap

Each milestone is demoable and teaches one core concept.

| # | Milestone | Concept learned | Service(s) |
|---|---|---|---|
| 1 | Upload a movie → MinIO + Postgres | presigned URLs, object storage, service shape | upload |
| 2 | Resumable / chunked uploads | large-file handling, multipart | upload |
| 3 | Enqueue via outbox → RabbitMQ; split → fan-out → fan-in skeleton (one worker) | async decoupling, outbox, chunked fan-out/fan-in | upload, transcode |
| 4 | **Worker pool: parallel chunk transcode → HLS** | **bounded concurrency** (prefetch/semaphore) | transcode |
| 5 | **Per-chunk retries (immediate, bounded to 3), idempotency, DB `dead`-state** | **failure handling** (interview gold) | transcode |
| 6 | Stream HLS to a browser player | adaptive bitrate streaming | stream |
| 7 | Observability + deploy (compose→k8s) | metrics, logs, ops | all |
| 8 | Load test: 100s of concurrent uploads | scale, backpressure, real numbers | all |

## 5. How we work

Feature-doc first, then function-by-function. Each feature gets a doc in
`features/`; each function is built in one of three modes (🟢 you write / 🟡 I
write, you study / 🔵 I explain, you attempt). Every architecture discussion is
recorded in DECISIONS.md.
