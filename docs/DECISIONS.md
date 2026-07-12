# Architecture Decision Log

This file records every architecture / design decision we make, with the *why*.
When you interview on this project, this is your script. Each entry: what we
decided, what we rejected, and why.

Format per entry:

- **Decision** — what we're doing
- **Context** — the problem / question that forced the choice
- **Options considered** — the alternatives and their tradeoffs
- **Chosen because** — the reasoning
- **Consequences** — what this now commits us to

---

## ADR-001 — Language: Go

- **Decision:** Build all services in Go.
- **Context:** Need a language that (a) makes high concurrency natural, (b)
  signals well for the backend↔DevOps roles being targeted, (c) deploys simply.
- **Options considered:**
  - **Go** — native goroutines/channels make worker pools & semaphores
    first-class; single static binary; the lingua franca of infra (Docker, k8s,
    Prometheus are all Go).
  - **Node/TS** — fast to build, weaker for demonstrating concurrency and infra.
  - **Python** — easiest to read, weakest concurrency story.
- **Chosen because:** The whole project is about *bounded concurrency + retries*;
  Go teaches that viscerally and matches the target roles.
- **Consequences:** Worker pools via goroutines + channels; `context.Context` for
  cancellation/timeouts; std `net/http` (or a light router) for services.

## ADR-002 — Deployment: local-first via docker-compose

- **Decision:** Everything runs locally first (docker-compose). Cloud/k8s is a
  later phase (milestone 7+), and we design so the swap is a config change.
- **Context:** Want to learn all the production concepts without cloud cost or
  slow iteration early on.
- **Options considered:** Real AWS (S3/SQS/ECS) from day one vs local clones.
- **Chosen because:** MinIO (S3-compatible), Postgres, and a local queue teach the
  *same concepts* at zero cost and faster iteration. We abstract storage & queue
  behind interfaces so moving to AWS later doesn't touch business logic.
- **Consequences:** Storage and queue access go through Go interfaces, never
  direct SDK calls in handlers. "Don't couple to your infra" is a design rule.

## ADR-003 — Uploads use presigned URLs (client uploads directly to storage)

- **Decision:** The upload service issues a *presigned URL*; the client uploads
  file bytes **directly to MinIO/S3**, not through our app.
- **Context:** Movie files are large (GBs). Streaming bytes through the app server
  wastes memory/bandwidth and makes the app a bottleneck.
- **Options considered:**
  - **Proxy through the app** — simple, but app holds the whole transfer; doesn't
    scale, ties up server resources.
  - **Presigned URL** — app only issues a short-lived signed URL; storage handles
    the bytes.
- **Chosen because:** Offloads the heavy transfer to storage; app stays light and
  scalable; this is how real systems (S3) do large uploads.
- **Consequences:** Upload becomes a 2-step handshake (init → get URL → client
  PUTs bytes → confirm). Sets up multipart/resumable uploads in milestone 2.

## ADR-004 — Architecture style: modular monorepo, independently deployable services

- **Decision:** 3 services (upload, transcode-worker, stream), each its own
  binary + container, but all in **one git repo** with shared libraries.
- **Context:** We want microservices' operational benefits (isolate failure,
  scale the transcode workers independently of the API) without the coordination
  cost of many repos while learning.
- **Options considered:**
  - **Single monolith** — simplest, but can't scale the heavy transcoder
    separately and doesn't teach service boundaries.
  - **Polyrepo microservices** — realistic at big scale, but premature: repo
    sprawl, shared-code duplication, CI overhead for a solo learner.
  - **Modular monorepo** — one repo, clear service boundaries, per-service
    binaries/containers.
- **Chosen because:** Gets microservice deployment benefits + the interview story
  ("I split by scaling axis"), while staying manageable. Honest about the tradeoff:
  full microservices day-one is usually over-engineering.
- **Consequences:** Services communicate via queue (async) + HTTP (sync), never by
  importing each other's internals. Shared code lives in `internal/` libs.

## ADR-005 — Repo layout: standard Go `cmd/` + `internal/`

> **SUPERSEDED by [ADR-010](#adr-010--code-organization-service-per-folder-monorepo-layered-controllerservicerepository).**
> We moved to a service-per-folder monorepo with a `shared/` package and explicit
> controller/service/repository layers.


- **Decision:** `cmd/<service>/main.go` for entrypoints, `internal/` for packages,
  `deploy/` for infra, `docs/` for docs.
- **Context:** Need a conventional Go structure that reviewers/interviewers
  recognize instantly.
- **Chosen because:** This is the idiomatic Go layout; `internal/` also enforces
  that packages can't be imported outside the repo. Storage & queue live behind
  interfaces here (makes [ADR-002](#adr-002--deployment-local-first-via-docker-compose) concrete).
- **Consequences:** One `main()` per deployable service; business logic never calls
  MinIO/Redis SDKs directly — only through `internal/storage` and `internal/queue`
  interfaces.

## ADR-006 — Job dispatch via a message broker (RabbitMQ), workers as competing consumers

- **Decision:** After an upload completes, transcode work is enqueued to a **message
  broker**; transcode-workers are **anonymous competing consumers**. Local broker =
  **RabbitMQ** (cloud = SQS). The upload-service never tells a specific worker to take
  a specific job.
- **Context:** Transcoding must happen asynchronously so a slow/failed transcode never
  blocks an upload. Many workers must share the backlog without ever double-processing
  a job, and must survive a worker crashing mid-job (transcodes run for minutes).
- **Options considered:**
  - **Push dispatch** (upload-service assigns a job to a chosen worker) — recreates a
    double-assignment race one layer up and makes the upload-service a single point of
    failure plus a worker registry. Rejected.
  - **DB-as-queue** (Postgres `SELECT … FOR UPDATE SKIP LOCKED` + lease/heartbeat) — no
    broker, transactional, but we hand-roll redelivery, visibility, and a DLQ.
  - **Message broker, competing consumers** (chosen) — the broker guarantees one
    in-flight delivery per message, auto-requeues on consumer disconnect, and provides a
    native dead-letter path.
  - **Broker choice** — RabbitMQ vs Redis Streams vs Redis lists. RabbitMQ has native
    dead-letter exchanges, per-message acks, and redelivery-on-disconnect built in; its
    ack/visibility model maps almost 1:1 to SQS.
- **Chosen because:** The project's core lesson is failure handling. RabbitMQ turns
  "redelivery + DLQ" from code we write into behaviour we configure *and must
  understand*; competing consumers give crash-safe, no-double-processing dispatch for
  free; and it maps cleanly to the SQS cloud target. Delivery is **at-least-once**, which
  *forces* idempotency — a feature, not a bug, for the learning goals.
- **Consequences:** Dispatch, redelivery, and lease/liveness move **out of the DB into
  RabbitMQ** — no more `SKIP LOCKED` or `locked_until`. The DB job table stays the
  authoritative **state** store. At-least-once delivery makes **idempotent transcode +
  ownership assertion** on write-back mandatory. Enqueue now spans DB + broker → see
  [ADR-007](#adr-007--reliable-enqueue-via-the-transactional-outbox-pattern). Retry policy
  is decided separately — immediate, bounded, DB-driven give-up, no broker DLQ. See
  [ADR-009](#adr-009--retry-policy-immediate-bounded-retries-db-driven-give-up-no-broker-dlq).

## ADR-007 — Reliable enqueue via the transactional outbox pattern

- **Decision:** `/complete` does **not** publish to RabbitMQ directly. In **one DB
  transaction** it flips the movie to `uploaded`, inserts the `transcode_jobs` row, and
  inserts an `outbox` row. A separate **relay** publishes unsent outbox rows to RabbitMQ
  and marks them sent.
- **Context:** Enqueue touches two systems — Postgres (job state) and RabbitMQ (the
  message). A naive "commit DB, then publish" can crash *in between*: the job exists but
  nothing will ever transcode it — an orphan. This is the **dual-write problem**.
- **Options considered:**
  - **Publish-then-write** or **write-then-publish** — either ordering loses or
    duplicates work on a crash between the two steps.
  - **Transactional outbox** (chosen) — the message is written to the DB in the *same
    transaction* as the state change, so it can never be lost; a relay forwards it to the
    broker at-least-once.
  - **2-phase commit across DB + broker** — heavyweight, poor tooling. Rejected.
- **Chosen because:** It makes the DB the single source of truth and the broker
  *eventually consistent* with it. A relay crash causes at most a **duplicate publish**,
  which idempotent workers already tolerate. Standard, well-understood pattern; strong
  interview signal.
- **Consequences:** Adds an `outbox` table and a relay. The relay runs as its **own
  standalone service** (`cmd/outbox-relay`) reading the same DB; it's stateless and claims
  rows with `SELECT … FOR UPDATE SKIP LOCKED` so multiple replicas never double-grab a
  row (a duplicate publish would be harmless anyway — workers are idempotent). Enqueue is
  **at-least-once** → reinforces the idempotency requirement from
  [ADR-006](#adr-006--job-dispatch-via-a-message-broker-rabbitmq-workers-as-competing-consumers).
  A reaper can re-drive rows stuck in `uploaded` as a backstop.

## ADR-008 — Distributed chunked transcoding (split → fan-out → fan-in)

- **Decision:** Transcoding is **distributed from the start**. On upload-complete the movie
  is **split** into keyframe-aligned source **chunks**; each chunk is transcoded
  independently and in parallel into all rungs' HLS segments; a **fan-in** step stitches
  and renumbers the segments, writes the playlists, then flips the movie to `ready`. A
  **parent** `transcode_jobs` row tracks the movie; **child** `chunk_tasks` rows are the
  units of work, each carrying a self-contained `payload`.
- **Context:** Movies are long. A single whole-movie job is serial (slow) and
  all-or-nothing on failure (a blip near the end re-does everything). We want parallel
  encode + per-chunk retry/checkpointing.
- **Options considered:**
  - **Whole-movie-per-job** — simplest, but serial and all-or-nothing retry. Rejected.
  - **Task per HLS output segment (~6 s)** — conflates the transcode unit with the output
    segment. Source keyframes rarely land on exact 6 s boundaries, so a cheap stream-copy
    split can't cut exact 6 s pieces; forcing it means per-piece re-encode (overhead) or
    ragged, non-uniform segments (worse ABR). Rejected **as the unit granularity**.
  - **Chunk-based distributed** (chosen) — split at source keyframes into chunks of a
    target duration; each chunk's *encode* forces a keyframe every 2 s → **uniform 6 s
    segments** regardless of chunk boundaries; fan-in stitches + renumbers. Decouples
    transcode granularity from output-segment granularity → correct, uniform, parallel,
    granular retry.
- **Chosen because:** it's how real distributed transcoders work — parallel encode,
  per-chunk checkpoint/retry, uniform output. Chunk size is a **tunable knob** (smaller =
  finer retry + more parallelism + more overhead/possible quality pulsing at boundaries;
  larger = fewer tasks).
- **Consequences:** Two-level job model (**parent** `transcode_jobs` + **child**
  `chunk_tasks` with a jsonb `payload` snapshot). Three work **phases**, each its own
  message type: `split`, `transcode-chunk`, `assemble`. **Fan-in completion is tracked in
  the DB** (`completed_chunks` vs `total_chunks`), not the broker. At-least-once delivery
  makes idempotency mandatory at *every* phase, and the `completed_chunks` increment must
  be tied to the chunk's status transition so a duplicate delivery can't double-count. The
  split step must produce keyframe-aligned chunks (ffprobe + segment-muxer stream-copy);
  assembly (playlist generation) is a distinct final step.

## ADR-009 — Retry policy: immediate, bounded retries, DB-driven give-up, no broker DLQ

- **Decision:** A failed chunk is retried **immediately** (no delay/backoff), up to
  `max_attempts` (**default 3**, configurable via env). On exhaustion the worker writes
  `error_msg`, sets `chunk_tasks.status=dead` and parent `phase=failed`, then **ACKs**.
  **No** RabbitMQ dead-letter exchange / DLQ.
- **Context:** Chunks fail (bad input, a transient storage blip). We need a bounded retry
  that provably converges, plus a durable, queryable record of terminal failure.
- **Options considered:**
  - **Delayed backoff** (TTL+DLX queues, or the delayed-message plugin) — survives
    transient outages, but adds broker machinery. Rejected for now for simplicity.
  - **Broker DLQ** for poison messages — parks the raw message for inspection/replay, but
    duplicates a *richer* DB record. Rejected as redundant.
  - **Immediate retry + DB dead-state** (chosen).
- **Chosen because:** `attempts` is already incremented **at claim**, so the DB counter
  bounds retries and converges to `dead` in ≤3 claims even under a nack/requeue loop. The
  DB `dead` row (status + `error_msg` + attempts + movie) is a richer, queryable record
  than a parked message — `WHERE status='dead'` finds failures, and a small admin
  re-enqueue replays them. Keeps the RabbitMQ topology minimal.
- **Consequences:** No retry queues, no DLX, no plugin. `max_attempts` configurable (env,
  default 3). **Trade-off accepted:** a transient dependency outage can burn all 3
  attempts in seconds and fail a job that backoff might have saved — revisit if it shows
  up in practice. Give-up is worker-driven (ACK after marking `dead`), so no message is
  left in the broker for a poison chunk.

## ADR-010 — Code organization: service-per-folder monorepo, layered (controller/service/repository)

- **Decision:** One monorepo (one `go.mod`), but **each service is a top-level folder**
  owning its full layer structure — `controller/` (gin handlers) → `service/` (business
  logic) → `repository/` (data access). Domain structs and cross-cutting infra live in a
  **`shared/`** package (`config`, `database`, `model`, `storage`, `queue`). Queries use
  **GORM**; the **repository sits behind an interface** injected into the service.
  **Migrations live in a separate repository** — no service contains or runs them.
- **Context:** ADR-005's `cmd/` + `internal/` technical split didn't express the
  service/business-logic boundaries clearly, and mixed a service's layers across generic
  packages. We want each service to read as a self-contained, layered unit, with clean
  dependency direction and testable business logic.
- **Options considered:**
  - **`cmd/` + `internal/` (ADR-005)** — idiomatic, but organizes by technical role, not
    by service; a service's controller/service/repository end up scattered. Superseded.
  - **Package-by-feature** (`internal/movie/{controller,service,repository}`) — good, but
    we chose service-folders at the top level so each *deployable* is self-evident.
  - **Service-per-folder + shared + layers** (chosen).
- **Chosen because:** each service directory is a complete, layered app that a reviewer
  can read top-down; the `controller → service → repository` flow is explicit; injecting
  the repository as an interface keeps services unit-testable with fakes; `shared/` avoids
  duplicating config/infra/models across services while one DB schema stays authoritative.
  Keeping migrations in their own repo decouples schema lifecycle from service deploys.
- **Consequences:** Still a monorepo (ADR-004 holds). Module paths are
  `…/movie-streamer/<service>/<layer>` and `…/shared/<pkg>`. GORM maps to tables but does
  **not** `AutoMigrate` — the separate migrations repo owns schema. Dependencies point one
  way (`controller→service→repository→db`); `model` is used by all and depends on nothing.
  `shared/storage` and `shared/queue` remain interfaces (ADR-002).

<!-- New decisions get appended here as we make them. -->

