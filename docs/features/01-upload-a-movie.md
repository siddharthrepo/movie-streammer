# Feature 01 — Upload a movie

> Status: **DRAFT — reviewing the design before writing any code.**
> Related decisions: [ADR-003 presigned uploads](../DECISIONS.md#adr-003--uploads-use-presigned-urls-client-uploads-directly-to-storage)

## 1. What & why

Let a client upload a movie file. The file lands in object storage (MinIO), and a
row describing it is written to the metadata DB (Postgres). This is the skeleton —
no transcoding yet, no resumable uploads yet. Goal: learn presigned URLs + object
storage + the basic service shape.

## 2. Flow (the request's journey)

```
1. Client:  POST /uploads            { filename, size, content_type }
2. Upload service:  validate input
3. Upload service:  create a movie row in Postgres  (status = "pending_upload")
4. Upload service:  ask MinIO for a presigned PUT URL for that object key
5. Upload service:  respond  { movie_id, upload_url }
6. Client:  PUT the file bytes directly to upload_url  (straight to MinIO)
7. Client:  POST /uploads/{movie_id}/complete
8. Upload service:  verify the object exists in MinIO
9. Upload service:  update the movie row  (status = "uploaded")
```

Key idea: our app never touches the file bytes (step 6 bypasses us). We only
orchestrate and record state.

## 3. The functions (what we'll build, in order)

> Each gets a mode when we start it: 🟢 you write / 🟡 I write, you study / 🔵 I explain, you attempt.

| # | Function (signature sketch) | Owns which step | Intent |
|---|---|---|---|
| 1 | `main()` | — | wire config, DB, storage client, router; start server |
| 2 | `storage.PresignPut(key, ttl) (url, error)` | 4 | ask MinIO for a signed PUT URL |
| 3 | `storage.ObjectExists(key) (bool, error)` | 8 | check the upload actually happened |
| 4 | `db.CreateMovie(m) (id, error)` | 3 | insert the pending row |
| 5 | `db.SetMovieStatus(id, status) error` | 9 | move the row to "uploaded" |
| 6 | `handleInitUpload(w, r)` | 2–5 | validate, create row, return presigned URL |
| 7 | `handleCompleteUpload(w, r)` | 8–9 | verify object, flip status |

## 4. Data & contracts

**DB — `movies` table (draft):**

| column | type | notes |
|---|---|---|
| id | uuid | primary key |
| filename | text | original name |
| object_key | text | where it lives in MinIO |
| size_bytes | bigint | claimed size |
| content_type | text | e.g. video/mp4 |
| status | text | pending_upload → uploaded |
| created_at | timestamptz | |

**API:**

```
POST /uploads
  req:  { "filename": "matrix.mp4", "size": 734003200, "content_type": "video/mp4" }
  res:  { "movie_id": "uuid", "upload_url": "https://minio/...signed..." }

POST /uploads/{movie_id}/complete
  res:  { "movie_id": "uuid", "status": "uploaded" }
```

## 5. Failure modes (the interview gold)

| Step | What can go wrong | What we do |
|---|---|---|
| 3 | DB insert succeeds but presign fails | row stuck in pending_upload — a later cleanup job can reap stale pending rows |
| 6 | Client never PUTs the bytes | row stays pending_upload forever → reaper deletes after TTL |
| 8 | Client calls /complete but object isn't in MinIO | reject; don't flip status |
| 8 | Uploaded size ≠ claimed size | (later) reject or flag; teaches validation |
| — | Client retries /complete twice | make it idempotent — flipping to "uploaded" twice is harmless |

## 6. Open questions to resolve before coding

_All resolved 2026-07-11 — see the decision log below._

- ~~Do we generate `object_key` server-side (uuid-based) or from the filename?~~ → **server-side uuid**.
- ~~Router: std `net/http` or a light lib like chi?~~ → **gin**.
- ~~Config: env vars via a small config struct?~~ → **yes, `.env` → `Config` struct**.

## 7. Decision log for this feature

- **2026-07-11 — `object_key` is generated server-side** (uuid-based), not derived from
  the client filename. *Why:* avoids collisions (two `matrix.mp4`s) and path-traversal
  attacks from attacker-controlled names.
- **2026-07-11 — HTTP router = gin.** *Why:* most widely recognized Go web framework
  (resume signal); batteries-included — routing, path params, JSON binding, middleware.
  *Rejected:* chi (lighter but less recognized), stdlib `net/http` (more boilerplate for
  binding/validation, even post-1.22).
- **2026-07-11 — Config via `.env` → a small `Config` struct** loaded at startup. *Why:*
  keeps secrets/endpoints out of code; the struct is the single typed source of config
  passed into `main()`. `.env` is gitignored; a committed `.env.example` documents keys.
