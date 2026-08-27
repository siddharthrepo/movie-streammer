# movie-streamer — Master Design

A video streaming platform whose novelty is **semantic scene seek**: mid-playback the user
types *"the scene where batman ties joker upside down"* and the player jumps to that
timestamp.

Upload → ffmpeg ladder → HLS → player is the substrate. Scene seek is the point.

---

## 1. How we build it

Start small, run it, watch where it breaks, fix it there. That is the whole method.

What is **not** negotiable is that the architecture stays correct while it grows. Small
does not mean wrong — it means fewer moving parts, each one shaped so the next part can
attach without tearing anything up.

So: no gates, no ceremony, no "phase 2 sign-off". Build the smallest honest version of the
loop, put real files through it, and let the failures tell us what to build next. The
destination in §6 is decided; how much of it exists at any moment is decided by what has
actually broken.

Nothing in the stated stack is compulsory. Kafka, microservices, MySQL — each earns its
place against a problem we actually have. We start with neither Kafka nor microservices,
and that is a design decision, not a shortcut.

**Concepts get taught as they come up.** Each load-bearing idea gets a note in
`docs/concepts/` when we hit it, written to be understood rather than skimmed. First one
is chunking (`01-chunking-in-rag.md`) — it is the main quality lever in the whole search
feature.

## 2. The seams

This is the part that has to be right on day one, and it is the whole reason later stages
are cheap. Every hard constraint we will hit has a known fix; the architecture's only job
is to keep those fixes to a **config swap instead of a rewrite**.

Four interfaces. Everything else is free to be naive.

```go
type Storage    interface { Put/Get/Presign/List }   // local fs → MinIO → S3
type Queue      interface { Publish/Subscribe/Ack }  // channel → Redis Streams → Kafka
type Embedder   interface { EmbedImage/EmbedText }   // local CPU → remote GPU endpoint
type VectorStore interface { Upsert/Query }          // Chroma → anything
```

The escape-hatch table — write it down now, execute it only when the pain is real:

| Constraint bites | Fix | Cost, because of the seam |
|---|---|---|
| Disk fills up | MinIO → S3 | env var |
| CPU too slow for indexing | Local models → GPU endpoint (Kaggle tunnel, Modal, RunPod) | env var + one adapter |
| RAM pressure | Move models out of process, behind the `Embedder` interface | already a network call |
| One worker too slow | In-process queue → Redis Streams → Kafka | one adapter, same handler code |
| One box is not enough | compose → k8s | config only, if 12-factor holds |
| Chroma outgrown | Swap `VectorStore` | one adapter; data is derived and rebuildable |

The corollary: **derived data must be rebuildable.** MySQL and object storage are truth;
vectors, captions, transcripts and HLS renditions can all be regenerated from source. That
is what makes every swap above safe.

---

## 3. First thing to build: the retrieval spike

Before any service exists, answer one question on one clip: **does semantic scene search
actually work?** Everything else is built in service of this feature, so it is the thing
worth knowing first.

Lives in **`poc/video-search/scene_search_poc.ipynb`**. Everything under `poc/` is
throwaway by design: what graduates into the pipeline is the *answer* — the numbers, the
model choices, the tuned thresholds — never the code.

**Runs in a Kaggle notebook****Runs in a Kaggle notebook** — free T4, ~30 GPU-hours/week. That takes the laptop's CPU
out of the picture entirely: we are testing whether the idea works, not whether this
machine can run it. What models fit on CPU is a later problem, and one with a known fix
(the `Embedder` seam).

One notebook, one clip, no services, no database:

1. Fetch a CC-BY clip (Blender open movies), 5–10 min with real physical action.
2. Shot detection → one keyframe per shot.
3. CLIP embeds keyframes; Whisper transcribes audio with word timestamps; a VLM writes a
   one-line caption per keyframe.
4. Group shots into scenes (§7) — this is the chunking decision, and the main quality lever.
5. Two Chroma collections; query hits both; Reciprocal Rank Fusion ranks the result.

**Write ~15 queries with their true timestamps while watching the clip.** Without labels
this degenerates into eyeballing plausible-looking output. With them, every later change
is measurable instead of arguable.

The useful output is the **ablation**: dialogue-only vs CLIP-only vs captions-only vs
fused. That table sizes everything downstream. If CLIP alone gets most of the way, the
expensive VLM pass dies and the CPU problem largely evaporates. If captions carry it, the
GPU escape hatch is not optional. Either answer is worth knowing before writing services.

## 4. Stage 1 — prototype pipeline

One machine. Two processes. **No Kafka, no microservices, no k8s.**

```
React (hls.js)  ──▶  api (Go)  ──▶  MySQL + local disk / MinIO
                        │
                        ├─ ffmpeg subprocess: transcode → HLS ladder
                        └─ Redis Stream ──▶ indexer (Python) ──▶ Chroma
```

- **`api` (Go)** — upload, movie metadata, serve HLS, proxy search. One binary, cobra
  subcommands (`serve`, `worker`, `migrate`).
- **`indexer` (Python)** — typer CLI. Consumes index jobs, runs the Stage-0 pipeline,
  writes scenes to MySQL and vectors to Chroma. Also serves `/search` in the same process
  for now — the split into `search-service` happens when indexing starts starving queries,
  which is a real, observable trigger.
- **Queue = Redis Stream.** Redis is already there for cache. Consumer groups, acks and a
  pending-entries list give at-least-once delivery and redelivery, which is everything we
  need. Kafka buys partition ordering and replay we do not yet have a use for. Behind the
  `Queue` seam, so switching later is one adapter.

Transcode in Stage 1 is **single-pass, whole-file** — ffmpeg produces the three-rung HLS
ladder in one shot. Chunked distributed transcode (§6) is Stage 2, triggered by the first
file big enough to make one-shot encoding unbearable.

**Stage 1 is done when:** a browser upload of a 10-minute clip results in a watchable
adaptive-bitrate stream, and a typed prompt seeks the player to the right moment.

That is the demo. Everything after it is engineering depth on top of a working demo,
which is the correct order — a hardened pipeline around a feature that does not work is
worth nothing.

---

## 5. Stage 2+ — harden, when it bites

Not a roadmap to execute top to bottom. A list of triggers.

| Trigger | Response |
|---|---|
| One-shot ffmpeg too slow on a feature-length file | Chunked split → fan-out → fan-in (§6) |
| Indexing starves live search | Split `search-service` out |
| CPU indexing unbearable | `Embedder` → remote GPU endpoint |
| Laptop disk full | `Storage` → S3 |
| Redis Streams strained, or replay needed | `Queue` → Kafka / Redpanda |
| Lost work on crash | Transactional outbox, idempotent handlers, DLQ |
| "Why is this slow?" is unanswerable | Prometheus, Grafana, trace IDs across async hops |
| Ready to prove it scales | k8s manifests, load test, numbers in the README |

The last two are what a platform-engineering interview actually digs into. They are not
optional polish — they are just not *first*.

---

## 6. Target architecture

The destination. Reached by the triggers above, not built up front.

```
browser ─▶ Traefik ─┬─▶ upload-service (Go)     gigabyte bodies, network-bound
                    ├─▶ catalog-service (Go)    read-heavy, Redis-cached, owns auth
                    ├─▶ search-service (Py)     latency-sensitive, p95 < 300ms
                    └─▶ nginx/MinIO             static HLS
                              │
                            queue
                    ┌─────────┴─────────┐
          transcode-service (Go)   ai-index-service (Py)
          split → fan-out → stitch  shots │ CLIP │ whisper │ VLM
                    │                     │
                MinIO (HLS)          Chroma + MySQL(scenes)
```

A service exists only if it sits on a **different scaling axis, runtime, or failure blast
radius** than its neighbours. Deliberately *not* split, and each is a talking point:

- **No `user-service`** — a login endpoint has no independent scaling axis.
- **No hand-written gateway** — Traefik does TLS, routing and rate limiting as config.
  Writing Go to replace a reverse proxy is the over-engineering tell.
- **No `playlist-service`** — HLS playlists are static files. That is nginx's job.

**Chunked transcode:** stream-copy split into ~30 s keyframe-aligned chunks → N parallel
ffmpeg workers → fan-in stitches and writes playlists. The completion counter increments
inside the `processing → done` row transition so redelivery cannot double-count; only the
worker that lands on `completed == total` triggers assembly.

**Indexing runs off the 360p rendition**, triggered as soon as that rung is ready — so
scene search is being built while 720p and 1080p are still encoding. Decoding 1080p just
to sample frames would waste the exact CPU that is the bottleneck.

---

## 7. Three granularities

The most important vocabulary here. Conflating these is the bug that ruins video pipelines.

| Term | Size | Purpose | Boundary |
|---|---|---|---|
| **chunk** | ~30 s | parallel transcode work | keyframe-aligned stream-copy split |
| **segment** | 6 s | HLS delivery | forced 2 s keyframes, listed in the playlist |
| **scene** | 2–10 s | semantic retrieval | shot boundary, content-detected |

One chunk holds many segments and many scenes. Scenes and segments never align, and are
never made to — a scene's `start_ms` becomes a player seek, and hls.js works out which
segment to fetch.

All three share **one timeline**: source presentation timestamps survive split, transcode
and stitch. That invariant is what makes an AI hit directly seekable, and it is also the
subtlest thing to get wrong — drift at chunk boundaries silently breaks scene seek. Stage
2 needs an automated check that probes the stitched output and asserts duration and
keyframe positions.

**Shot ≠ scene, and this matters for quality.** A shot is one camera take, ~3 s. "Batman
ties joker upside down" is a dozen shots cutting back and forth over 90 s. Retrieval must
group adjacent shots into a scene and return the *start*, or the player drops the user
into the middle of the action.

---

## 8. Data model

MySQL is truth. Chroma holds only derived data and can be dropped and rebuilt from MySQL
plus object storage — which is precisely why a second store is acceptable.

| Table | Purpose |
|---|---|
| `users` | auth |
| `movies` | title, status, source key, duration |
| `renditions` | one row per rung, playlist key, status |
| `scenes` | `start_ms`, `end_ms`, caption, transcript, thumbnail key |
| `transcode_jobs`, `chunk_tasks` | Stage 2 — parent/child work tracking |
| `outbox` | Stage 2 — reliable publish |

`movies.status`: `created → uploading → uploaded → transcoding → ready`. Index status is
tracked separately: a movie is **watchable before it is searchable**, and the UI says so.

---

## 9. Conventions

Service-per-folder monorepo. Each Go service layers `route → controller → service →
repository`, dependencies pointing one way, repository behind an injected interface. No
central `Config` struct — `global/constants.go` loads `.env` once in `init()` and exposes
typed package vars.

Every service is a CLI with subcommands, not a bare `main` — `serve`, `worker`, `migrate`,
plus one-off operational commands, so a backfill never requires a code change. Go uses
**cobra**; Python uses **typer** (cobra is Go-only; typer is the same idea).

`migrations/` is top-level and shared; no service runs migrations itself.

**No comments in source files.** Explanation lives in these docs and in commit messages.

### Where structs live

**There is exactly one `structs.go` per service, and it lives in `global/`.** Every struct in
the service is declared there — request bodies, response bodies, error shapes, and the domain
types packages pass between each other. One file, one place to look.

`model/` is the only other home for type declarations, and only for database entities, because
gorm tags, `TableName()` and lifecycle hooks belong with the entity.

**The single exception:** a struct whose fields are typed by a third-party SDK stays beside its
implementation. Not a style preference — two language constraints force it:

1. Go requires methods to be declared in their type's package, so moving the struct moves its
   entire implementation with it.
2. `global` is imported by every package. Putting `*s3.Client` in it makes `model`, `route` and
   `controller` all transitively depend on the AWS SDK.

`storage.s3Storage` is the current example: it holds `*s3.Client` and `*s3.PresignClient`, so it
is declared at the top of `storage/s3.go`. Its public vocabulary — `Part`, `PresignedPart`,
`ObjectInfo` — lives in `global/structs.go` like everything else.

---

## 10. Decisions

| # | Decision | Why |
|---|---|---|
| 001 | Staged: POC → prototype → harden | Prove the novelty before building infrastructure around it |
| 002 | Design the seams on day one | Every later constraint becomes a config swap, not a rewrite |
| 003 | Derived data must be rebuildable | What makes those swaps safe |
| 004 | Stage 0 on Kaggle GPU | Tests the idea without testing the laptop |
| 005 | Ground-truth labels + ablation in the POC | Otherwise it is eyeballing, and the ablation sizes Stage 1 |
| 006 | Redis Streams first, not Kafka | Consumer groups and acks are all Stage 1 needs; Kafka's ordering and replay have no use yet |
| 007 | Fuse visual + dialogue with RRF | Rank fusion needs no calibration between incomparable vector spaces |
| 008 | Chroma for vectors | Python-native, derived-only, so no backup or migration story |
| 009 | Shot-detect before embedding | ~150x fewer frames; what makes CPU-only viable at all |
| 010 | Index the 360p rendition | Decoding 1080p to sample frames wastes the bottleneck resource |
| 011 | chunk ≠ segment ≠ scene | Three real granularities; forcing alignment breaks ABR or retrieval |
| 012 | Legally clean test corpus | Private eval may use any clip on disk; anything *shipped* — repo, README, demo — is Blender open movies + public domain. Media is gitignored |
| 013 | nginx at the edge, no custom gateway | Do not write code to replace configuration. Static service set makes Traefik's auto-discovery unnecessary |
| 014 | Watchable before searchable | Indexing is minutes-to-hours; playback must not wait on it |
| 015 | Presigned direct-to-S3 upload | A 50 GB file must never stream through our own pods |
| 016 | Transcode work unit is (chunk, quality) | Per-quality workers cannot resume; ffmpeg has no mid-file restart point |
| 017 | Source archived, never deleted | Deleting the mezzanine makes every derived artifact permanent and unfixable |
| 018 | MySQL rows as the job queue | `SELECT ... FOR UPDATE SKIP LOCKED` gives competing consumers with no broker |
| 019 | Shared MySQL, disjoint table ownership | The boundary that matters is who writes which tables, not how many DB servers run |
| 020 | Plan claims in the JWT | Entitlement checks must not put a network hop on the playback path |
| 021 | Object storage is env-driven, LocalStack by default | No creds configured → LocalStack endpoint + path-style addressing + test creds. Creds present → real S3. Same code path, the `Storage` seam decides at construction |

---

## 11. Hardware

12 cores, **15 GB RAM (~7 GB actually free)**, Intel iGPU — no CUDA — ~40 GB free disk.
Docker 29.6.1 (snap), ffmpeg 6.1.1, Go 1.25.8, Python 3.12.3, Node 24.

**RAM is the binding constraint, not disk.** The Stage 6 target stack — Kafka, MySQL,
MinIO, Chroma, plus Python services holding CLIP, Whisper and a VLM — sums to roughly
8 GB against 7 GB free. The full stack and an indexing run cannot coexist here.

Consequences, all already handled by the staging:
- Stage 0 sidesteps it entirely by running on Kaggle.
- Stage 1 is two processes, not eight, and Redis instead of a JVM broker.
- Compose profiles keep `core` and `ai` separable.
- The `Embedder` seam is the standing escape hatch to borrowed GPU.

Develop against **5–15 minute clips**. Run one full-length film end to end exactly once,
late, to have the number for the README.
