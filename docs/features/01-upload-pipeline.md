# Feature 01 — Upload Pipeline

Take a movie file from a browser and turn it into playable HLS at three qualities, without
ever streaming the bytes through our own services, and without losing more than 30 seconds
of work when a pod dies.

**Owned by:** `catalog-service` (Go) for state and API, `transcode-service` (Go) for work.
**Depends on:** MySQL, S3 (MinIO locally), ffmpeg.
**Not in scope:** playback, auth, AI indexing. Those are separate features.

---

## 1. The shape of it

```
browser ──1── POST /uploads              ──▶ catalog-service ──▶ MySQL (movies, upload_jobs)
        ◀─2── presigned S3 multipart URLs

browser ──3── PUT parts ──────────────────▶ S3   (bytes never touch our services)

browser ──4── POST /uploads/{id}/complete ──▶ catalog-service ──▶ job state = uploaded

transcode-service ──5── claim chunk (SKIP LOCKED + lease) ──▶ MySQL
                  ──6── ffmpeg chunk ──▶ S3 segments
                  ──7── mark chunk done

catalog-service   ──8── all chunks done ──▶ write playlists, state = completed
```

Steps 5–7 run on N workers in parallel. Nothing coordinates them beyond the database.

---

## 2. Why chunks, not qualities

The obvious decomposition is one worker per quality. It cannot satisfy our resume
requirement: a single ffmpeg pass over a whole file has **no intermediate state**, so a pod
dying at 80% loses everything. It also decodes the source three times.

So the unit of work is **(chunk, quality)** — a ~30 second slice of video at one rendition.

```
90 min film → 180 chunks × 3 qualities = 540 independent work items
```

- **Resumable** — completed items are rows. A dead worker costs ≤30s of work.
- **Horizontally scalable** — 540 items, add workers to go faster. Per-quality caps at 3.
- **Honest progress** — `312/540` is a real number, not an estimate.
- **Idempotent** — output keys are deterministic, so re-running an item overwrites rather
  than duplicates.

### Chunks must be keyframe-aligned

A chunk boundary that lands mid-GOP produces a segment that cannot be decoded standalone.
Consequences are silent and nasty: A/V drift at seams, and timestamps that no longer match
the source — which breaks scene seek later.

So the mezzanine is cut only at keyframes, and the transcode forces keyframes on a fixed
grid:

```
-force_key_frames expr:gte(t,n_forced*2)     keyframe every 2s
CHUNK_SECONDS = 30                            multiple of segment duration
SEGMENT_SECONDS = 6                           multiple of keyframe interval
```

`30 % 6 == 0` and `6 % 2 == 0`. This is load-bearing arithmetic: chunk *i* produces exactly
segments `[i*5 .. i*5+4]`, so **no concatenation step is needed**. Each worker writes its
own final segments at their final names, and the playlist is generated at the end from the
chunk table.

See `docs/concepts/` for the granularity vocabulary — chunk (work unit) is not segment
(delivery unit) is not scene (retrieval unit).

---

## 3. Data model

All tables owned by `catalog-service`. `transcode-service` writes only `transcode_chunks`.

### `movies`
| column | type | notes |
|---|---|---|
| id | CHAR(36) PK | UUID, used in every S3 key |
| title | VARCHAR(255) | |
| description | TEXT | |
| duration_ms | BIGINT NULL | filled by ffprobe after upload |
| status | VARCHAR(32) | `draft`, `ready`, `failed` |
| created_at / updated_at | DATETIME | |

### `upload_jobs`
| column | type | notes |
|---|---|---|
| id | CHAR(36) PK | |
| movie_id | CHAR(36) FK | |
| source_key | VARCHAR(512) | `movies/{movie_id}/source/original.mp4` |
| source_size | BIGINT NULL | |
| s3_upload_id | VARCHAR(255) NULL | S3 multipart id |
| state | VARCHAR(32) | see §4 |
| chunk_count | INT DEFAULT 0 | |
| error | TEXT NULL | |
| created_at / updated_at | DATETIME | |

### `transcode_chunks`
| column | type | notes |
|---|---|---|
| id | BIGINT PK AUTO | |
| job_id | CHAR(36) FK | |
| chunk_index | INT | 0-based |
| quality | VARCHAR(8) | `360p`, `480p`, `720p` |
| start_ms / end_ms | BIGINT | source time range |
| state | VARCHAR(16) | `pending`, `leased`, `done`, `failed` |
| lease_owner | VARCHAR(64) NULL | worker id |
| lease_expires_at | DATETIME NULL | |
| attempts | INT DEFAULT 0 | |
| error | TEXT NULL | |

`UNIQUE (job_id, chunk_index, quality)` — the idempotency guarantee.
`INDEX (state, lease_expires_at)` — the claim query's access path.

### `renditions`
| column | type | notes |
|---|---|---|
| id | BIGINT PK AUTO | |
| movie_id | CHAR(36) FK | |
| quality | VARCHAR(8) | |
| playlist_key | VARCHAR(512) | |
| bandwidth | INT | for the master playlist |
| segment_count | INT | |

---

## 4. Job state machine

```
pending_upload ──(client confirms)──▶ uploaded ──(probe+plan)──▶ processing
                                                                     │
                          ┌──────────────────────────────────────────┤
                          ▼                                          ▼
                     completed                                    failed
```

| state | meaning | who moves it |
|---|---|---|
| `pending_upload` | presigned URLs issued, bytes not confirmed | catalog |
| `uploaded` | source object exists in S3 | catalog, on `/complete` |
| `processing` | probed, chunks planned, workers running | catalog |
| `completed` | all chunks done, playlists written | catalog |
| `failed` | terminal; `error` populated | either |

Transitions are guarded by the current state in the `WHERE` clause, so a concurrent or
replayed request cannot move a job backwards.

---

## 5. Claiming work without a queue

We do not need Kafka for this. A database row *is* a job, and MySQL 8 gives us safe
competing consumers directly:

```sql
SELECT id FROM transcode_chunks
WHERE state = 'pending'
   OR (state = 'leased' AND lease_expires_at < NOW())
ORDER BY job_id, chunk_index
LIMIT 1
FOR UPDATE SKIP LOCKED;
```

`SKIP LOCKED` makes concurrent workers step over rows another transaction holds, so two
workers never claim the same chunk. The claim sets `state='leased'`, `lease_owner`,
`lease_expires_at = NOW() + 10 min`.

**This is the resume mechanism.** A pod that dies holds a lease that simply expires; the row
returns to the pool and another worker redoes ≤30s of work. There is no crash handler, no
cleanup job, no distributed lock — recovery is the absence of a heartbeat.

A worker renews its lease periodically while ffmpeg runs. Exceeding `MAX_ATTEMPTS` marks the
chunk `failed`, which fails the job.

---

## 6. API

| method | path | purpose |
|---|---|---|
| POST | `/api/v1/uploads` | create movie + job, return presigned part URLs |
| POST | `/api/v1/uploads/{id}/complete` | client confirms all parts uploaded |
| GET | `/api/v1/uploads/{id}` | poll status |
| DELETE | `/api/v1/uploads/{id}` | abort, clean up S3 |

`GET` response is what the frontend polls:

```json
{
  "id": "...", "movie_id": "...", "state": "processing",
  "progress": { "done": 312, "total": 540, "percent": 57.8 },
  "qualities": { "360p": "done", "480p": "processing", "720p": "processing" }
}
```

Progress is computed from `transcode_chunks` in MySQL. Redis may cache this response later
(§Phase 8) but **MySQL is the source of truth** — Redis is evictable and cannot be trusted
with the state of a 90-minute job.

---

## 7. S3 layout

One bucket. Prefixes, not buckets — buckets are a global namespace with account limits and
per-bucket IAM, they are not folders.

```
s3://movie-streamer-media/
  movies/{movie_id}/
    source/original.mp4          ← lifecycle → Glacier after 30d, never deleted
    360p/segment_00000.ts ... index.m3u8
    480p/...
    720p/...
    master.m3u8
```

Keys use `movie_id`, never title — titles collide, contain unicode, and change.

### Where the bucket lives

Selection is env-driven and defaults to LocalStack, so a clone runs with no AWS account:

| env | LocalStack (default) | real S3 |
|---|---|---|
| `S3_ENDPOINT` | `http://localhost:4566` | empty — SDK resolves AWS |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | `test` / `test` | real credentials |
| `S3_USE_PATH_STYLE` | `true` | `false` |

**Path style is not optional locally.** Real S3 addresses buckets as
`bucket.s3.region.amazonaws.com`; that hostname cannot resolve against `localhost:4566`, so
LocalStack requires `endpoint/bucket/key` instead. Getting this wrong produces DNS errors
that look like network problems rather than configuration.

The rule: **absence of credentials selects LocalStack.** No separate `USE_LOCALSTACK` flag to
forget to flip — if you have not configured AWS, you get the local emulator.

One consequence to expect: presigned URLs generated against LocalStack carry a
`localhost:4566` host. Anything consuming them — browser, curl, a container on another
network — must be able to reach that address.

**The source is archived, not deleted.** Deleting it makes every derived artifact permanent:
no new rendition, no re-run after a transcode bug, no mezzanine for AI indexing. A lifecycle
rule costs ~$1/TB/month and preserves the ability to rebuild.

---

## 8. Failure modes

| failure | detection | response |
|---|---|---|
| client abandons upload | `pending_upload` older than 24h | sweeper aborts S3 multipart, deletes job |
| worker dies mid-chunk | lease expiry | another worker reclaims |
| ffmpeg fails on a chunk | non-zero exit | retry to `MAX_ATTEMPTS`, then fail job |
| corrupt source | ffprobe fails | job → `failed` immediately, before planning chunks |
| duplicate `/complete` | state guard in `WHERE` | idempotent no-op |
| S3 write partially done | deterministic keys | overwrite on retry |
| segment count mismatch | verify before playlist | job → `failed`, do not publish a broken playlist |

---

## 9. Implementation phases

Each phase is independently runnable and demoable, and each stays under the 500-line rule.
No phase leaves the repo in a state where nothing works.

| # | phase | what exists at the end | ~lines |
|---|---|---|---|
| 1 | **catalog skeleton** | cobra `serve`, gin router, gorm, config from env, `/healthz`, migrations, `movies` CRUD | ~450 |
| 2 | **presigned upload** | `POST /uploads` returns real S3 part URLs; `/complete` verifies object exists; `upload_jobs` table | ~400 |
| 3 | **probe + plan** | ffprobe on the source, `duration_ms` filled, `transcode_chunks` rows generated, state → `processing` | ~300 |
| 4 | **worker skeleton** | `transcode-service` with cobra `work`, claim loop with SKIP LOCKED, lease renewal, no ffmpeg yet | ~350 |
| 5 | **single-quality transcode** | worker runs ffmpeg for 360p chunks, writes segments to S3, marks done | ~400 |
| 6 | **playlists** | per-rendition `index.m3u8` + `master.m3u8` written when chunks complete; job → `completed`; **a movie plays in a browser** | ~350 |
| 7 | **full ladder** | 480p and 720p, bandwidth metadata, ABR switching works | ~200 |
| 8 | **hardening** | retries, sweeper for abandoned uploads, segment verification, Redis cache on the poll endpoint, lifecycle rule | ~450 |

**Phase 6 is the milestone that matters** — that is the first point where a file goes in one
end and plays out the other. Phases 7 and 8 make it good; phase 6 makes it real.

Order is deliberate: single quality before the ladder, because the ladder is a loop over
something already proven, and debugging three renditions at once is three times the surface
for the same lesson.

---

## 10. Decisions specific to this feature

| # | decision | why |
|---|---|---|
| U1 | presigned direct-to-S3 upload | bytes never cross our pods; S3 handles part retry |
| U2 | work unit is (chunk, quality) | the only decomposition that makes resume possible |
| U3 | keyframe-aligned chunks, `30 % 6 == 0` | lets chunks emit final segments — no concat step |
| U4 | MySQL rows as the queue, `SKIP LOCKED` | competing consumers without a broker; revisit if claim latency bites |
| U5 | lease expiry as crash recovery | no heartbeat protocol, no distributed lock |
| U6 | source archived, never deleted | derived data must stay rebuildable |
| U7 | MySQL is state truth, Redis is cache | Redis is evictable; a 90-min job's state is not |
