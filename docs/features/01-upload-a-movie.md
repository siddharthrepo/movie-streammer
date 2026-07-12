# Feature 01 — Reliably upload a large movie (multipart + resumable)

> Status: **DRAFT — redesigned for reliable 1–10 GB uploads; no code yet.**
> Related decisions: [ADR-003 presigned uploads](../DECISIONS.md#adr-003--uploads-use-presigned-urls-client-uploads-directly-to-storage)
> Supersedes the earlier single-PUT skeleton (which explicitly deferred resumable uploads).

## 1. What & why

Let a client upload a **1–10 GB** movie file **reliably** and land it in object storage
(MinIO), with a `movies` row recording its state. "Reliably" is the whole point: a single
`PUT` of 10 GB is fragile — one dropped connection restarts the entire transfer, and long
single transfers hit timeouts. So we use **S3/MinIO multipart upload**: the file is split
into parts, each part is uploaded independently (its own presigned URL), a failed part is
retried on its own, and an interrupted upload **resumes** by re-sending only the missing
parts.

The upload-service **never touches the file bytes** (ADR-003) — presigned per part, so
parts go straight from the client to MinIO. The service only orchestrates and records
state.

## 2. Flow (the journey)

```
1. Client:  POST /uploads { filename, size, content_type }
2. Service: validate; create movies row (pending_upload) with a server-side object_key;
            CreateMultipartUpload on MinIO -> upload_id; store it on the row
3. Service: respond { movie_id, upload_id, part_size, part_count }
4. Client:  split the file into part_count parts of part_size bytes
5. Client:  POST /uploads/{movie_id}/parts/urls { part_numbers:[...] }
6. Service: return presigned PUT URLs for those parts
7. Client:  PUT each part directly to MinIO (parallel); keep each part's ETag
                - a part fails?      -> retry just that part
                - a URL expired?     -> ask for a fresh one (step 5)
8. Client:  POST /uploads/{movie_id}/complete
9. Service: ListParts(upload_id) -> verify all part_count parts present (no gaps)
            -> CompleteMultipartUpload(manifest) -> flip movies -> uploaded

RESUME (client crashed / disconnected between 4 and 8):
   GET /uploads/{movie_id}/status -> { uploaded_parts:[...], part_size, part_count }
   client re-requests URLs for the MISSING parts only (step 5) and continues.
```

```
                POST /uploads ─────────────▶┌────────────────┐
   ┌────────┐  ◀── {upload_id, part_size} ──│ upload-service │──▶ CreateMultipartUpload
   │ client │   POST /parts/urls ───────────│                │──▶ presign part URLs        [ MinIO ]
   │        │  ◀── {presigned part URLs} ────│                │                                 ▲
   │        │                                └───────┬────────┘                                 │
   │        │   PUT part 1 ─────────────────────────────────────────────────────────────────────┤
   │        │   PUT part 2 ─── (direct to MinIO, parallel, per-part retry) ─────────────────────▶ │
   │        │   PUT part N ─────────────────────────────────────────────────────────────────────┘
   │        │   POST /complete {ETags} ─────▶ upload-service ──▶ CompleteMultipartUpload ──▶ object assembled
   │        │   GET /status (resume) ───────▶ upload-service ──▶ ListParts ──▶ which parts exist
   └────────┘
```

Key idea: the app orchestrates a **session** (the `upload_id`); the heavy bytes go
client→MinIO directly, one retryable/resumable part at a time.

## 3. API

```
POST /uploads
  req: { "filename": "matrix.mkv", "size": 9663676416, "content_type": "video/x-matroska" }
  res: { "movie_id": "uuid", "upload_id": "…", "part_size": 52428800, "part_count": 185 }

POST /uploads/{movie_id}/parts/urls
  req: { "part_numbers": [1, 2, 3] }
  res: { "urls": { "1": "https://minio/…sig…", "2": "…", "3": "…" }, "expires_in": 3600 }

GET  /uploads/{movie_id}/status
  res: { "status": "pending_upload", "part_size": 52428800, "part_count": 185,
         "uploaded_parts": [1, 2, 5] }

POST /uploads/{movie_id}/complete
  req: {}                       # service assembles the manifest from ListParts
  res: { "movie_id": "uuid", "status": "uploaded" }

POST /uploads/{movie_id}/abort        (optional; lifecycle rule is the backstop)
  res: { "movie_id": "uuid", "status": "aborted" }
```

## 4. Data model — `movies`

| column | type | notes |
|---|---|---|
| id | uuid | pk |
| filename | text | original name |
| object_key | text | server-side uuid key in MinIO |
| size_bytes | bigint | claimed total size |
| content_type | text | e.g. video/x-matroska |
| status | text | `pending_upload → uploaded` (+ `aborted`) |
| upload_id | text | the MinIO multipart session id |
| part_size | bigint | bytes per part (last part may be smaller) |
| created_at / updated_at | timestamptz | |

> **No `upload_parts` table** (leaning on MinIO `ListParts` as the source of truth for
> which parts landed — see open question #2). Per-part progress is not duplicated in our DB.

## 5. The functions (build order)

> Modes: 🟢 you write / 🟡 I write, you study / 🔵 I explain, you attempt.
> Layered: controller (HTTP) → service (orchestration) → repository (DB) + storage (MinIO).

| # | Function (sketch) | Layer | Intent |
|---|---|---|---|
| 1 | config load + DB connect + router wiring | main | boot the service |
| 2 | `storage.CreateMultipartUpload(key)` → uploadID | storage | start a multipart session |
| 3 | `storage.PresignPart(key, uploadID, partNo, ttl)` → url | storage | per-part presigned PUT URL |
| 4 | `storage.ListParts(key, uploadID)` → []partNo | storage | resume: which parts exist |
| 5 | `storage.CompleteMultipart(key, uploadID, parts)` | storage | assemble the object |
| 6 | `storage.AbortMultipart(key, uploadID)` | storage | cancel + free storage |
| 7 | `repo.CreateMovie` / `repo.SetStatus` / `repo.GetByID` | repository | movie row lifecycle |
| 8 | `service.InitUpload` | service | create row + start session + compute parts |
| 9 | `service.PresignParts` / `service.Status` / `service.Complete` | service | orchestration |
| 10 | `controller` handlers + routes | controller | parse/validate, call service |

## 6. Failure & resume modes (the interview gold)

| Situation | What we do |
|---|---|
| A part upload fails mid-transfer | client retries **just that part**; the other parts are untouched |
| A presigned part URL expires | client re-requests a fresh URL for that part (`/parts/urls`) |
| Client crashes / disconnects | resume: `GET /status` (via `ListParts`) → re-upload only missing parts → complete |
| `/complete` but not all parts uploaded | service sees a gap in `ListParts` (count < `part_count`) → reject with the missing part numbers; **never** completes a truncated object |
| `/complete` called twice | idempotent: if already `uploaded` (object exists), return success |
| Upload started, never completed | **abandoned multipart uploads linger and cost storage** → bucket lifecycle rule `AbortIncompleteMultipartUpload` after N days (+ optional reaper of stale `pending_upload` rows) |
| Uploaded size ≠ claimed size | (optional) verify total on complete; reject/flag |

## 7. Open questions (to lock before coding)

- ~~**#1 Part size**~~ → **RESOLVED**: fixed server-dictated **`part_size` = 50 MB**
  (1 GB → 20 parts, 10 GB → ~200 parts; well under the 10 000-part cap). Last part smaller.
- ~~**#2 Resume tracking**~~ → **RESOLVED**: MinIO **`ListParts`** is the source of truth;
  **no `upload_parts` table**. The service builds the complete manifest from `ListParts`
  (client `/complete` sends no ETags) and verifies all `part_count` parts are present
  before completing, so a gap can never assemble a truncated object.
- ~~**#3 Part-URL delivery**~~ → **RESOLVED**: **on-demand batches** — client calls
  `POST /parts/urls { part_numbers:[...] }` for the parts it's about to upload (and only
  the missing ones on resume). Fresh URLs, few round-trips.
- ~~**#4 Cleanup of abandoned uploads**~~ → **RESOLVED**: **both** — a MinIO lifecycle rule
  `AbortIncompleteMultipartUpload` (storage backstop) **and** a reaper that aborts stale
  `pending_upload` rows (keeps the DB clean).
- ~~**#5 Integrity**~~ → **RESOLVED**: **ETags only** (per-part ETag + TLS) for now; SHA-256
  per-part checksums are a later upgrade if needed.
- ~~**#6 Presigned URL TTL**~~ → **RESOLVED**: **1 hour** per part URL.
- ~~**#7 Parallelism / part-count cap**~~ → **RESOLVED**: **no server-side concurrency cap**
  (parts upload directly to MinIO — not ours to control); document a suggested ~4–8 parallel
  parts. 50 MB parts stay under the 10 000-part ceiling up to 500 GB, far above our range.

## 8. Decision log for this feature

- **2026-07-11** — `object_key` generated **server-side** (uuid), not from the filename
  (collisions + path-traversal safety).
- **2026-07-11** — HTTP router = **gin**.
- **2026-07-11** — Config via **`.env` → `Config` struct**; `.env` gitignored, `.env.example` committed.
- **2026-07-12** — Upload is **multipart + resumable** (S3/MinIO multipart), not a single
  presigned PUT — required for reliable 1–10 GB uploads. Supersedes the single-PUT skeleton.
- **2026-07-12** — Part size = fixed **50 MB** (server-dictated).
- **2026-07-12** — Resume/progress via MinIO **`ListParts`**; no `upload_parts` table.
  Service assembles the complete manifest from `ListParts` and verifies no gaps before
  `CompleteMultipartUpload`. Client `/complete` sends no ETag body.
- **2026-07-12** — Part URLs delivered **on-demand in batches** (`POST /parts/urls`), not
  all upfront — avoids presigned-URL expiry on long uploads; natural for resume.
- **2026-07-12** — Abandoned-upload cleanup = **both** a MinIO `AbortIncompleteMultipartUpload`
  lifecycle rule (storage backstop) **and** a reaper of stale `pending_upload` rows.
- **2026-07-12** — Integrity = **ETags only** for now (SHA-256 per-part later if needed).
- **2026-07-12** — Presigned part URL TTL = **1 hour**.
- **2026-07-12** — **No server-side concurrency cap** (direct-to-MinIO); suggest ~4–8
  parallel parts. 50 MB parts stay under the 10 000-part cap up to 500 GB.
