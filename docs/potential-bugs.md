# Potential bugs & scaling limits

A running list of things expected to break, and the scale at which each should
break. This is the scorecard for load testing: the plan is to build the base
(upload → transcode → streaming → frontend), then drive it with automated load
from small to very large, and use the numbers to confirm, refute, or re-rank
every entry below.

An entry is only worth having if it predicts something measurable. "This might
be slow" is not an entry. "Claim p99 exceeds 200 ms past ~500k pending rows
because the ORDER BY cannot use `idx_chunk_claim`" is.

## Method

1. Write the entry **before** the load test, with a predicted breaking point.
2. Run the load test. Record the actual number.
3. A wrong prediction is the useful outcome — it means the mental model was
   wrong somewhere more interesting than the code.
4. Fix, re-run, record the new number. Move the entry to Resolved with both.

Predictions here are estimates from reading the code, not measurements. Any
number not marked `measured` is a guess.

## Status legend

| status | meaning |
|---|---|
| `read` | found by reading code; not yet observed running |
| `predicted` | consequence of a constant or a design choice; needs a load test to confirm |
| `measured` | reproduced under load, with numbers |
| `fixed` | resolved, with before/after numbers |

---

## Open

| # | area | issue | status | shows up at |
|---|---|---|---|---|
| B1 | transcode | chunk ordering causes head-of-line blocking between movies | `read` | 2+ concurrent movies |
| B2 | transcode | worker connection pool sized like a web server | `predicted` | ~7 worker pods |
| B3 | transcode | claim query cannot use an index for its `ORDER BY` | `predicted` | ~500k pending rows |
| B4 | upload | presigned part URLs expire mid-upload on slow links | `read` | 2 GB on <5 Mbps uplink |
| B5 | upload | abandoned multipart uploads are never cleaned up | `read` | first abandoned upload |
| B6 | catalog | `movies.status` never reaches `ready` | `read` | already true |
| B7 | catalog | concurrent `plan` runs duplicate ffprobe work | `read` | 2 concurrent planners |
| B8 | transcode | `attempts` does not distinguish retryable from terminal errors | `read` | first transient S3 error |
| B9 | transcode | no backpressure — one user can fill the queue | `read` | 1 user, many movies |
| B10 | transcode | idle polling cost scales with pod count | `predicted` | ~100 workers |
| B11 | storage | S3 request rate is per-prefix | `predicted` | ~3.5k PUT/s to one movie |

---

### B1 — chunk ordering causes head-of-line blocking

`transcode-service/repository/chunk_repository.go`

```sql
ORDER BY job_id, chunk_index, quality
```

`job_id` is a UUID, so it sorts arbitrarily but *stably*. Workers therefore
drain one movie's entire chunk set before starting the next.

With 10 movies of 720 work items each, the movie whose UUID sorts last does not
begin transcoding until roughly 90% of all work is done. At ~3 CPU-hours per
movie that is a multi-hour wait before the user sees any progress at all.

It also contradicts ADR-014 (watchable before searchable): one movie becomes
fully playable while nine have not started, instead of all ten becoming
playable at 360p early.

**Fix:** order by rendition rank first, then chunk index, then job:

```sql
ORDER BY quality_rank, chunk_index, job_id
```

All movies then advance together, and every movie is watchable at 360p after
roughly a sixth of the total work. Needs a `quality_rank` column or a `FIELD()`
expression, plus an index that can serve the new ordering.

**Load test:** upload 10 movies at once, plot per-movie completion percentage
over time. Broken ordering gives ten staircases in series; fixed ordering gives
ten roughly parallel ramps.

### B2 — worker connection pool sized like a web server

`MYSQL_MAX_OPEN_CONNS=25` is shared by every service. It is a reasonable number
for a gin server handling concurrent requests, and wrong for a worker pod whose
3 goroutines each hold at most one connection at a time.

MySQL's default `max_connections` is 151. At 25 connections per pod, connection
exhaustion arrives at ~7 pods — well before any queue limit, and while total
CPU utilisation is still low.

**Fix:** per-service pool sizing. A worker pod needs roughly
`concurrency + 2`, not 25.

**Load test:** scale worker pods up while watching
`SHOW STATUS LIKE 'Threads_connected'` and the error rate. The failure should
appear as connection errors, not slowness — which is what makes it easy to
misdiagnose as a network problem.

### B3 — claim query cannot use an index for its `ORDER BY`

`idx_chunk_claim (state, lease_expires_at)` serves the `WHERE`, but the
`ORDER BY job_id, chunk_index, quality` is not a prefix of any index, so MySQL
sorts the matching rows on every claim.

At 7k pending rows this is invisible. The question is where it stops being
invisible — the guess is somewhere around 500k rows, but this is exactly the
kind of number that should be measured rather than reasoned about.

**Load test:** seed the table with 10k / 100k / 1M pending rows and record
claim p50/p99 at each. `EXPLAIN` should show `Using filesort`. Fixing B1
changes this ordering anyway, so measure after B1, not before.

### B4 — presigned part URLs expire mid-upload

`UPLOAD_PRESIGN_TTL_SECONDS=3600`, and all 32 part URLs for a 2 GB file are
issued at once, at initiate time. A client on a 5 Mbps uplink needs ~55 minutes
for the file — and any part not started within the hour gets a 403.

The failure mode is bad: the upload dies most of the way through, and the parts
already in S3 keep costing money because the multipart upload is never
completed or aborted (see B5).

**Fix:** an endpoint to re-presign remaining parts, and a client that requests
URLs in batches rather than all up front.

**Load test:** throttle a client to 2 Mbps and upload 2 GB.

### B5 — abandoned multipart uploads are never cleaned up

Nothing aborts a multipart upload whose client walked away. S3 bills for
uploaded parts of incomplete multipart uploads indefinitely, and the
`upload_jobs` row sits in `pending_upload` forever.

Phase 8 has the sweeper. The S3 lifecycle rule
(`AbortIncompleteMultipartUpload`) is the belt-and-braces version and costs one
line of bucket config.

### B6 — `movies.status` never reaches `ready`

`global.StatusReady` is declared and referenced nowhere. Movies stay `draft`
forever. Phase 6 should set it when the playlists are written — noted here so
it is not mistaken for a runtime bug later.

### B7 — concurrent `plan` runs duplicate ffprobe work

`PlanPending` lists jobs in state `uploaded`, then probes each. The state
transition is guarded by `MarkProcessing`, so the *writes* are safe, but two
planners started at the same time will both run ffprobe against the same
sources before one loses the race.

Harmless today because `plan` is invoked manually. It becomes real the moment
planning is scheduled or runs in more than one replica. The claim pattern
already used for chunks is the fix.

### B8 — `attempts` does not distinguish retryable from terminal errors

Every failure increments `attempts`, and 3 strikes is permanent. A corrupt
source and three transient S3 timeouts are treated identically, so a chunk can
be permanently failed by a bad afternoon on the network.

**Fix:** classify errors at the executor boundary — transient errors requeue
without consuming an attempt (the mechanism already exists, added for shutdown
in ADR-023); only deterministic failures count.

### B9 — no backpressure

Nothing limits how much of the queue one job or one user can occupy. One user
uploading 100 movies enqueues 72,000 work items ahead of everyone else. B1
makes this worse, but fixing B1 does not fix this — fair ordering still lets
one user own most of the rows.

**Fix:** per-user concurrency caps at plan time, or weighted fair queueing in
the claim.

**Load test:** one user uploads 50 movies while a second uploads 1. Measure the
second user's time-to-first-playable.

### B10 — idle polling cost scales with pod count

Each idle worker issues one claim query every `WORKER_IDLE_BACKOFF_MS` (3s).
33 workers idle = 11 queries/sec that return nothing. Cheap now; at 300 workers
it is 100 queries/sec of pure waste, which is the same order as the claim
threshold the queue design was sized against.

**Fix:** longer idle backoff with jitter, or a notification channel to wake
workers only when work exists. Note that a broker here would be a *latency*
optimisation, not a correctness one — MySQL stays the source of truth (ADR-007
… ADR-018).

### B11 — S3 request rate is per-prefix

Segments live under one prefix per movie. S3 allows ~3,500 PUT/s per prefix.
Ten movies in parallel spread across ten prefixes, so this is not a near-term
concern — it becomes one if many workers ever hammer a single movie's prefix,
e.g. a re-transcode of one long film with high worker counts.

---

## Not yet observable

These need streaming and the frontend to exist before they can be tested at
all. Listed so the load-test plan accounts for them.

| area | question to answer |
|---|---|
| streaming | how many concurrent viewers does one pod serve before p99 segment latency degrades? |
| streaming | does plan validation on the playback path add measurable latency (ADR-020)? |
| catalog | what is the read QPS ceiling before Redis is genuinely needed rather than assumed? |
| nginx | connection limits and buffering behaviour with many slow HLS clients |
| end-to-end | time from upload complete to first playable byte, as a function of queue depth |

---

## Resolved

| # | issue | found | fix | numbers |
|---|---|---|---|---|
| B0 | shutdown consumed a retry attempt, so 3 rolling deploys permanently failed a healthy chunk | phase 4 lease-theft test | `Requeue` with `GREATEST(attempts - 1, 0)` (ADR-023) | 2 chunks interrupted at SIGTERM, `attempts` back to 0 |
