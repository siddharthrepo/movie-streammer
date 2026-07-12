# Feature 02 — Transcode a movie into HLS renditions (distributed, chunked)

> Status: **DRAFT — design agreed; no code yet.**
> Related decisions: [ADR-003 presigned uploads](../DECISIONS.md#adr-003--uploads-use-presigned-urls-client-uploads-directly-to-storage),
> [ADR-006 broker/RabbitMQ](../DECISIONS.md#adr-006--job-dispatch-via-a-message-broker-rabbitmq-workers-as-competing-consumers),
> [ADR-007 outbox](../DECISIONS.md#adr-007--reliable-enqueue-via-the-transactional-outbox-pattern),
> [ADR-008 distributed chunked transcoding](../DECISIONS.md#adr-008--distributed-chunked-transcoding-split--fan-out--fan-in),
> [ADR-009 retry policy](../DECISIONS.md#adr-009--retry-policy-immediate-bounded-retries-db-driven-give-up-no-broker-dlq)
> Supersedes the scratch notes in `text.txt`.

## 1. What & why

Once a movie is `uploaded`, convert it into **HLS adaptive-bitrate renditions**
(1080p/720p/360p) so players can stream it and switch quality on the fly.

The work is **distributed from day one** (ADR-008): the movie is split into
keyframe-aligned **chunks**, chunks are transcoded **in parallel** by many workers, and a
**fan-in** step stitches the output back together. This gives parallel encode (a long
movie finishes in minutes, not serially) and **per-chunk retry** (a failure re-does one
chunk, not the whole movie).

Dispatch is via **RabbitMQ**; workers are anonymous **competing consumers** (not push
targets). The DB is the **authoritative state store**; RabbitMQ owns delivery/redelivery.

### Vocabulary (these are NOT the same thing)
- **chunk** — the *parallel work unit*: a keyframe-aligned slice of the source
  (~target 30 s). One row in `chunk_tasks`. This is what a worker picks up.
- **segment** — the *6 s HLS output file* a player downloads. Uniform 6 s, produced by
  forcing a keyframe every 2 s **during encode**, independent of chunk boundaries.
- One chunk produces **several** segments (per rung). See ADR-008 for why chunk ≠ segment.

## 2. Flow (end to end)

```
        ┌─────────┐  1. POST /uploads             ┌────────────────┐
        │ client  │ ──────────────────────────────▶│ upload-service │
        │         │◀─ 2. 202 {movie_id, PUT url} ──│                │
        │         │  3. PUT bytes (presigned)      └───────┬────────┘
        │         │ ────────────────────────┐              │ 5. tx: movies=uploaded,
        │         │  4. POST /complete       │              │    transcode_jobs(splitting),
        │         │ ─────────────────────────┼─────────────▶│    outbox{split}
        │         │◀─ 200 uploaded ──────────┘              ▼
        └─────────┘                                    [ Postgres ]  ◀── state, counts ──┐
                                                        movies / jobs / chunk_tasks /     │
                                        6. relay ──────▶ outbox                           │
                                        publishes         │                               │
                                            ▼             │                               │
   ┌────────────── [ RabbitMQ ]  split | chunk | assemble  queues ────────────────────┐   │
   │                     │                     │                        │             │   │
   ▼ 7. SPLIT            ▼ 9. TRANSCODE CHUNK   │                        ▼ 12. ASSEMBLE│   │
┌──────────────┐   ┌──────────────────────┐    │                  ┌──────────────────┐│   │
│ split worker │   │  chunk worker  (×N)  │◀───┘ fan-out           │  assemble worker ││   │
│ ffprobe +    │   │ download chunk →     │                        │ build playlists  ││   │
│ keyframe cut │   │ ffmpeg → 3 rungs of  │  11. tx: chunk=done,    │ (media+master) → ││   │
│ → N chunks   │   │ 6s segments → upload │  completed_chunks++;    │ movie=ready      ││   │
└──────┬───────┘   └──────────┬───────────┘  if last → outbox{assemble} └────┬─────────┘│   │
       │ 8. insert N          │ 10. upload segments                         │ 13. ack   │   │
       │ chunk_tasks+outbox    ▼                                            ▼           │   │
       └────────────────▶ [ MinIO ] raw / chunks / hls  ◀────────────────────────────────┘   │
                                                          14. GET .m3u8 (only if ready) ──────┘
                                        ┌────────────────┐
   player ─────────────────────────────▶│ stream-service │ ── redirect ──▶ [ MinIO ] / CDN
                                        └────────────────┘
```

### Phase A — Upload (synchronous, client-facing)
1. `POST /uploads {filename, size, content_type}`.
2. Validate; generate a **server-side `object_key`** (uuid); insert a `movies` row
   `status=pending_upload`; return **202** + a presigned PUT URL.
3. Client PUTs the bytes **straight to MinIO** (ADR-003).
4. Client calls `POST /uploads/{id}/complete`.
5. **Trust-but-verify** the object exists. Then **in one DB tx**: `movies → uploaded`,
   insert a `transcode_jobs` parent row `phase=splitting`, and insert an `outbox` row
   `{type: split, job_id}`. Commit; return 200.

### Phase B — Split (fan-out producer)
6. The relay publishes the `split` message to RabbitMQ (ADR-007).
7. A **split worker** claims it, runs **ffprobe** to read duration + keyframe timestamps,
   and chooses chunk boundaries **on keyframes** grouped to a target ~30 s. It
   stream-copies (no re-encode → fast) the source into N chunk files in MinIO under
   `movies/{id}/chunks/`.
8. **In one DB tx**: insert N `chunk_tasks` rows (each with a self-contained `payload`),
   set `transcode_jobs.total_chunks = N`, `phase = transcoding`, and insert N `outbox`
   rows `{type: chunk, chunk_task_id}`. Commit. — Idempotent: keyed on `job_id`, a
   re-run finds chunks already created and skips.

### Phase C — Transcode chunks (fan-out consumers, the heavy path)
9. RabbitMQ delivers `chunk` messages to workers with `prefetch=K` (bounded concurrency).
   A **chunk worker** claims its `chunk_tasks` row: `status=processing`, `worker_id`,
   `attempts++`, `started_at`.
10. Download the one chunk file (small), run **ffmpeg** → all 3 rungs of **6 s segments**
    (keyframes forced every 2 s, aligned across rungs), upload them to
    `movies/{id}/hls/{rung}/` using the **global segment numbering** from `payload`.
11. **In one DB tx**: `UPDATE chunk_tasks SET status='done' … WHERE id=? AND worker_id=?
    AND status='processing'`. If 1 row changed, `UPDATE transcode_jobs SET
    completed_chunks = completed_chunks + 1 … RETURNING completed_chunks, total_chunks`.
    If `completed_chunks == total_chunks`, insert `outbox {type: assemble, job_id}` **in
    the same tx**. Commit, then **ACK**. (Tying the increment to the status transition is
    what makes duplicate deliveries safe — see §8.)

### Phase D — Assemble (fan-in)
12. An **assemble worker** consumes `assemble`. It generates, per rung, a media playlist
    (`index.m3u8` listing every segment in order with exact `EXTINF` durations) and the
    **master playlist** (the menu of rungs + their `BANDWIDTH`). Uploads them under
    `movies/{id}/hls/`.
13. **In one DB tx**: `transcode_jobs.phase = ready`, `movies.status = ready`. ACK.
    (Idempotent: regenerating playlists overwrites deterministically.)

### Phase E — Stream (synchronous, viewer-facing — milestone 6)
14. Player `GET /movies/{id}/master.m3u8` → stream-service, **served only if
    `status=ready`**. **Hybrid delivery (control plane / data plane):** stream-service
    itself serves the small **playlists** (where auth + short-lived signed segment URLs
    are injected), and the player fetches the **segments directly from MinIO/CDN** via
    presigned GET — bytes are never proxied through the app. This is the same split
    YouTube/Hotstar use; it preserves HLS's static-file/CDN scaling. See §9 #5.

## 3. Where each reliability mechanism lives

| Concern | Mechanism |
|---|---|
| No double-processing a chunk | **RabbitMQ** — one unacked delivery per message |
| Crashed worker's chunk returns | **RabbitMQ** — connection drops → auto-requeue |
| Stale worker can't overwrite | **DB** ownership assertion `WHERE worker_id = me` |
| Never serve a half-transcode | **DB** — movie flips `ready` only after fan-in |
| Duplicate delivery can't double-count | **DB** — increment tied to the `processing→done` transition (§8) |
| Only one worker triggers fan-in | **DB** — atomic `+1 RETURNING`; only the row that hits `== total` enqueues `assemble` |
| Retry on failure | **requeue immediately** (no delay); bounded to **3 attempts** by the DB `attempts` counter |
| Poison chunk gives up | worker marks `chunk_tasks.status=dead` + `error_msg`, fails parent `phase=failed`, then ACKs |
| Enqueue without losing work | **outbox** (ADR-007), used at split, fan-out, and fan-in |

## 4. Data model

**`movies`** — durable entity:

| column | type | notes |
|---|---|---|
| id | uuid | pk |
| filename | text | original name |
| object_key | text | raw original in MinIO |
| size_bytes | bigint | claimed size |
| content_type | text | e.g. video/mp4 |
| status | text | `pending_upload → uploaded → processing → ready` |
| output_prefix | text | HLS tree root once `ready` |
| created_at / updated_at | timestamptz | |

**`transcode_jobs`** — parent, one per movie, tracks the fan-in:

| column | type | notes |
|---|---|---|
| id | uuid | pk |
| movie_id | uuid | fk → movies |
| phase | text | `splitting → transcoding → assembling → ready` \| `failed` |
| source_key | text | raw original |
| output_prefix | text | HLS root |
| total_chunks | int | set by the split step |
| completed_chunks | int | atomically incremented at fan-in |
| error_msg | text | |
| created_at / updated_at / started_at / finished_at | timestamptz | |

**`chunk_tasks`** — child, the parallel unit of work:

| column | type | notes |
|---|---|---|
| id | uuid | pk |
| job_id | uuid | fk → transcode_jobs |
| chunk_index | int | order (queryable) |
| status | text | `queued → processing → done` \| `failed` \| `dead` |
| attempts / max_attempts | int | **incremented at claim** → poison converges to `dead` |
| worker_id | text | `pod:startup_uuid` — ownership assertion |
| **payload** | **jsonb** | self-contained work spec (snapshot — see §5) |
| error_msg | text | |
| created_at / updated_at / started_at / finished_at | timestamptz | |

**`outbox`** — reliable publish (ADR-007): `id, payload jsonb, created_at, published_at`.

> No `locked_until` anywhere — RabbitMQ owns lease/visibility (ADR-006).

## 5. The `payload` — self-contained chunk spec (snapshot)

Written by the split step so a worker can process a chunk cold, reading nothing else.
Snapshotted (not a reference) so a retried chunk reproduces the exact same work even if
config changes later — reinforcing idempotency.

```json
{
  "source_chunk_key": "movies/abc123/chunks/chunk-007.ts",
  "chunk_index": 7,
  "first_segment_number": 35,
  "renditions": [
    { "name": "1080p", "w": 1920, "h": 1080, "v": "5000k", "a": "128k" },
    { "name": "720p",  "w": 1280, "h": 720,  "v": "2800k", "a": "128k" },
    { "name": "360p",  "w": 640,  "h": 360,  "v": "800k",  "a": "96k"  }
  ],
  "output_prefix": "movies/abc123/hls/"
}
```

`first_segment_number` is how fan-out stays orderable: chunk 7's segments are numbered
from where chunk 6's ended, so the media playlist just lists `seg000…segM` in order.
Queryable fields (`status`, `chunk_index`, `worker_id`) stay as real columns; the spec
lives in `payload`.

## 6. Message contracts (all thin — the DB is the truth)

```json
{ "type": "split",    "job_id": "uuid" }
{ "type": "chunk",    "chunk_task_id": "uuid" }
{ "type": "assemble", "job_id": "uuid" }
```

## 7. State machines

```
movies:  pending_upload → uploaded → processing → ready

parent (transcode_jobs.phase):
         splitting → transcoding → assembling → ready
                          │            │
                          └──── failed ─┘   (a chunk went dead, or assembly failed)

child (chunk_tasks.status):
         queued → processing → done
            ▲          │
            └─ retry ──┤→ failed (attempts < max)
                       └→ dead   (attempts ≥ max → DLQ, terminal)
```

Only `movies.status = ready` is streamable. Terminal chunk states are never re-delivered.

## 8. Failure modes (the interview gold)

| Where | What goes wrong | What we do |
|---|---|---|
| 5 | `/complete` but object missing in MinIO | reject; don't flip status |
| 5→6 | DB commits, dies before publish | **outbox** relay publishes on restart; never orphaned |
| 7 | split fails / dies midway | retried; idempotent (re-probe + re-split overwrite chunk files) |
| 9–11 | chunk worker dies mid-transcode | unacked → RabbitMQ requeues; segments overwrite deterministically; partial output never served (movie not `ready`) |
| 11 | **duplicate delivery** (at-least-once) | `done` transition is guarded `WHERE status='processing'`; a 0-row update ⇒ skip the counter increment ⇒ **no double-count** |
| 11 | two workers both think they're "last" | `+1 … RETURNING` is atomic; exactly one sees `== total_chunks` and enqueues `assemble` |
| 10 | ffmpeg fails (bad chunk) | mark `failed`; **nack/requeue immediately**; the redelivery re-claims and does `attempts++` |
| — | poison chunk, `attempts ≥ 3` | worker marks `chunk_tasks.status=dead` + `error_msg`, sets parent `phase=failed`, then **ACKs** (message consumed) |
| 12 | assemble fails / dies | retried; idempotent (playlists regenerated + overwritten) |
| 11/12 | stale worker finishes late | ownership assertion `WHERE worker_id=me` → no-op |

## 9. Open questions

- ~~**#1 Rendition ladder**~~ → **RESOLVED**: 1080p (5 Mbps) / 720p (2.8 Mbps) / 360p
  (800 kbps); **6 s** segments; keyframes forced every **2 s** aligned across rungs; never
  upscale.
- ~~**#2 Chunk target size**~~ → **RESOLVED**: **~30 s**, snapped to source keyframes so
  boundaries fall on segment boundaries (multiples of 6 s); configurable.
- ~~**#3 Retry policy / backoff**~~ → **RESOLVED** ([ADR-009]): **no delay** between
  retries; requeue immediately, **max 3 attempts** (bounded by the DB `attempts` counter,
  incremented at claim; `max_attempts` is **configurable via env**, default **3** if
  unset). On the 3rd failure: write `error_msg`, mark chunk `dead`, fail the parent job.
  **No** plugin, retry queues, or broker DLQ — the DB `dead` state is the record.
- ~~**#4 Relay location**~~ → **RESOLVED**: its **own standalone service**
  (`cmd/outbox-relay`) reading the same Postgres and publishing to RabbitMQ. Stateless;
  claims rows with `SELECT … FOR UPDATE SKIP LOCKED` so multiple replicas never
  double-grab a row.
- ~~**#5 Stream serving**~~ → **RESOLVED**: **hybrid** (control plane / data plane split,
  as YouTube/Hotstar do). stream-service serves the small **playlists** itself (auth +
  signed URLs injected) and the player fetches **segments directly from MinIO/CDN** via
  presigned GET — never proxied through the app. DRM / edge-token auth is a later
  cloud-phase upgrade. (matters at milestone 6)
- ~~**#6 Split step home**~~ → **RESOLVED**: its **own async step** — `/complete` just
  writes `outbox{split}` and returns fast; a `split` message is handled by the
  transcode-worker (`handleSplit`), not inline in the HTTP request. Same async-decoupling
  reason we don't transcode in the upload path; keeps ffmpeg/ffprobe off the upload-service.

## 10. The functions (build order)

> Modes: 🟢 you write / 🟡 I write, you study / 🔵 I explain, you attempt.

| # | Function (sketch) | Phase | Intent |
|---|---|---|---|
| 1 | `queue.Publish` / `queue.Consume` | all | RabbitMQ behind the queue interface (ADR-002) |
| 2 | `outbox.Enqueue(tx, msg)` / `outbox.Relay()` | 5,6,8,11 | reliable publish |
| 3 | `split.Probe(src)` → duration + keyframes | 7 | ffprobe wrapper |
| 4 | `split.Plan(keyframes, target)` → chunk boundaries | 7 | group keyframes to ~30 s |
| 5 | `split.Cut(src, boundaries)` → chunk files | 7 | stream-copy segment muxer |
| 6 | `db.CreateChunkTasks(tx, job, chunks)` | 8 | insert N children + counts + outbox |
| 7 | `db.ClaimChunk(id, workerID)` | 9 | processing + attempts++ + ownership |
| 8 | `transcode.RunChunk(chunk, payload, outDir)` | 10 | build + exec the ffmpeg HLS command |
| 9 | `storage.Download` / `storage.UploadDir` | 10 | chunk in, segments out |
| 10 | `db.CompleteChunk(tx, id, workerID)` → bool lastOne | 11 | guarded done + atomic counter |
| 11 | `assemble.Playlists(job)` | 12 | media playlists + master |
| 12 | `db.MarkReady` / `db.MarkFailed` | 12,fail | terminal transitions |
| 13 | `worker.handleChunk` / `handleSplit` / `handleAssemble` | 7–13 | orchestrators |

## 11. Decision log for this feature

- **2026-07-11** — Message broker (RabbitMQ), competing consumers, not push. → [ADR-006].
- **2026-07-11** — Reliable enqueue via transactional outbox. → [ADR-007].
- **2026-07-11** — **Distributed chunked transcoding** (split → fan-out → fan-in),
  parent `transcode_jobs` + child `chunk_tasks`, from day one. → [ADR-008].
- **2026-07-11** — `chunk` = parallel work unit (keyframe-aligned, ~30 s); `segment` = 6 s
  HLS output (forced 2 s keyframes). One chunk → many segments; they are different
  granularities on purpose.
- **2026-07-11** — `payload` is a **snapshot** (self-contained), jsonb; queryable fields
  stay as columns.
- **2026-07-11** — Fan-in tracked in the DB (`completed_chunks`/`total_chunks`); the
  counter increment is tied to the chunk's `processing→done` transition so duplicate
  deliveries can't double-count, and only the worker that hits `== total` enqueues assembly.
- **2026-07-11** — Rendition ladder: 1080p/720p/360p, 6 s segments, 2 s aligned keyframes,
  never upscale.
- **2026-07-12** — Chunk target size **~30 s**, snapped to source keyframes on segment
  boundaries (multiples of 6 s); configurable.
- **2026-07-12** — Retry policy (→ [ADR-009]): **no delay**, requeue immediately,
  **max 3 attempts** (bounded by the DB `attempts` counter; `max_attempts` configurable
  via env, default 3). On exhaustion: `error_msg` + chunk `dead` + parent `phase=failed`.
- **2026-07-12** — **No RabbitMQ DLQ / dead-letter exchange.** The DB `dead` row (status +
  `error_msg` + attempts + movie) is the record; give-up is worker-driven (ACK after
  marking `dead`).
- **2026-07-12** — Outbox relay runs as its **own standalone service** (`cmd/outbox-relay`),
  reading the same DB. Stateless; claims rows with `SKIP LOCKED` for multi-replica safety.
- **2026-07-12** — Stream serving = **hybrid** (control plane / data plane, à la
  YouTube/Hotstar): stream-service serves playlists (auth + signed URLs); segments fetched
  **directly from MinIO/CDN** via presigned GET, never proxied. DRM/edge-token = later.
- **2026-07-12** — Split runs as its **own async step** (`outbox{split}` → transcode-worker
  `handleSplit`), not inline in `/complete` — keeps the upload path fast and ffmpeg-free.

[ADR-006]: ../DECISIONS.md#adr-006--job-dispatch-via-a-message-broker-rabbitmq-workers-as-competing-consumers
[ADR-007]: ../DECISIONS.md#adr-007--reliable-enqueue-via-the-transactional-outbox-pattern
[ADR-008]: ../DECISIONS.md#adr-008--distributed-chunked-transcoding-split--fan-out--fan-in
[ADR-009]: ../DECISIONS.md#adr-009--retry-policy-immediate-bounded-retries-db-driven-give-up-no-broker-dlq
