# Learning Plan — Mastery Gates

The rule: a phase is **not done when the code works — it's done when you can
answer its questions cold, without looking anything up.** Working code you can't
explain is worthless in an interview. These questions are what you'll actually be
asked.

Mark each ✅ only when you can answer it out loud, in your own words, unprompted.

---

## Week 0 — Streaming fundamentals (know the OUTPUT before building the pipeline)

You must understand *what the pipeline produces* before building the pipeline.
Everything downstream (transcoder, storage layout, stream service) exists to
create and serve HLS. So we start here.

- ✅ **Why does HLS scale trivially?** Segments are static files; the CDN caches
  them; origin barely gets hit; there's no stateful per-viewer video connection —
  it's just HTTP GETs of `.ts`/`.m4s` files.
- ✅ **What does the player switch on?** Client-side decision: measured bandwidth
  (and buffer health) vs. the `BANDWIDTH` attribute of each variant in the master
  playlist. The server is passive — it just offers the menu.
- ✅ **Why ~6-second segments?** Tradeoff: shorter = faster quality switching but
  more HTTP requests; longer = fewer requests but coarser switching. Also affects
  startup latency and is coupled to the keyframe/GOP interval (segments must start
  on a keyframe).

Remaining Week 0 gates (to complete before milestone 6, ideally understood now):

- ⬜ **Master playlist vs. media playlist — what's in each?** Master = the menu of
  variants (each with a `BANDWIDTH` + resolution, pointing to a media playlist).
  Media playlist = the ordered list of segment files for one specific rendition.
- ⬜ **What does the transcoder actually emit for one movie?** One master playlist
  + N media playlists (one per rendition: 1080p/720p/480p…) + the segment files
  for each. This is the storage layout the pipeline must produce and the stream
  service must serve.

---

## Week 1 — Upload service

The service that accepts uploads and records metadata. Gates:

- ⬜ **Why does our app never touch the file bytes?** Large files would bottleneck
  the app on memory/bandwidth. Presigned URLs let the client upload directly to
  storage; the app only orchestrates and records state.
- ⬜ **What is a presigned URL and why is it safe?** A time-limited, cryptographically
  signed permission slip to do one specific storage operation (PUT this key). It
  expires; it can't be reused for other keys/operations.
- ⬜ **How does the app know an upload actually happened?** It can't trust the client.
  Either the client calls `/complete` and the app *verifies against storage*
  (trust-but-verify), or storage fires an event. The DB is a state machine that can
  get stuck in `pending_upload` — a reaper cleans that up.
- ⬜ **Why is `/complete` idempotent?** Clients retry. Flipping `pending → uploaded`
  twice must be harmless.

---

## Later phases (gates filled in as we reach them)

- **Week 2 — Resumable/chunked uploads:** Why multipart? What makes an upload
  resumable? How do you resume after a network failure without re-sending bytes?
### Week 3 — The queue / async decoupling

- ⬜ **Why decouple upload from transcode?** If you transcode *synchronously* in the
  upload request, the request blocks for minutes (times out, ties up connections), a
  transcode failure fails the upload, and you can't scale the transcoder independently.
- ⬜ **What is the dual-write problem, and how does the outbox solve it?** Enqueue touches
  two systems (DB + broker); a crash between "commit DB" and "publish" orphans the job.
  The outbox writes the message *in the same DB transaction*, and a relay forwards it —
  so the message can never be lost, only duplicated (which idempotency absorbs).
- ⬜ **Why competing consumers, not push dispatch?** Push (upload-service assigns a job to
  a chosen worker) recreates a double-assignment race and makes the upload-service a SPOF
  + a worker registry. The broker hands each message to exactly one consumer instead.
- ⬜ **What does at-least-once delivery force on you?** Idempotency — the same message may
  be delivered twice, so processing it twice must be harmless.

### Week 4 — Worker pool (core)

- ⬜ **Why a *bounded* pool, not one goroutine per job?** ffmpeg pins CPU cores;
  unbounded concurrency thrashes/OOMs. Concurrency ≈ cores.
- ⬜ **What is RabbitMQ prefetch, and how does it create backpressure?** It caps unacked
  messages per consumer, so a busy worker stops pulling new work until it acks.
- ⬜ **What happens when jobs arrive faster than workers drain?** The queue grows — that's
  the buffer doing its job; you add worker replicas. The upload path stays unaffected.

### Week 5 — Failure handling (interview gold)

- ⬜ **What is exponential backoff + jitter, and why did we consciously skip it?** Backoff
  stops hammering a recovering dependency; jitter de-synchronizes retries. We chose
  *immediate* bounded retries instead (simpler, no broker machinery) — the accepted
  trade-off is that a transient outage can burn all 3 attempts fast. Know when you'd add
  backoff back (ADR-009).
- ⬜ **What makes the transcode idempotent, and why is it mandatory here?** Write output,
  flip to `ready` as the last atomic step; a re-run overwrites deterministically and a
  half-written result is never served. Mandatory because delivery is at-least-once.
- ⬜ **Why a DB `dead`-state instead of a broker DLQ?** The DB row already holds a richer,
  queryable record (status + `error_msg` + attempts + movie) than a parked message;
  `WHERE status='dead'` finds failures and a small admin re-enqueue replays them. Know
  when a broker DLQ *would* earn its keep.
- ⬜ **How is the retry count bounded without a broker feature?** `attempts` is incremented
  **at claim**, so even a nack/requeue loop converges to `dead` in ≤3 claims.
- ⬜ **What race does the ownership assertion (`WHERE worker_id = me`) prevent?** A stale
  worker (whose chunk was requeued) finishing late and stomping the new owner's result.
- **Week 6 — Stream service:** (Week 0 gates realized) serving master/media playlists
  + segments, byte-range, why the CDN sits in front.
- **Week 7 — Observability + deploy:** What do you measure (queue depth, job latency,
  failure rate)? Why those?
- **Week 8 — Load test:** What actually falls over first under 100s of concurrent
  uploads, and how do you know?
