# Concept 02 — Running jobs without a message broker

Written while designing the upload pipeline. The question was: 540 transcode work items,
several workers, pods that die. Do we need Kafka?

No. And understanding *why* is worth more than the answer, because "we added Kafka" is a
weak interview answer and "we used the database until it stopped being enough, here is the
number where that happens" is a strong one.

---

## A queue is three guarantees

Strip away the products and any job queue provides exactly three things:

1. **Hand each item to exactly one worker** (no double work)
2. **Give it back if the worker dies** (no lost work)
3. **Let workers scale independently of producers**

That's it. Kafka provides them. So does a table with three columns, and for a long time the
table is the better choice — because it needs no extra process, participates in the same
transaction as your data, and is queryable with tools you already have.

---

## Guarantee 1: exactly one worker

The naive version has a race:

```sql
SELECT id FROM transcode_chunks WHERE state = 'pending' LIMIT 1;
UPDATE transcode_chunks SET state = 'leased' WHERE id = ?;
```

Two workers run the `SELECT` simultaneously, both see the same row, both claim it. The work
is done twice.

Adding `FOR UPDATE` fixes correctness but destroys throughput: worker B *blocks* waiting for
worker A's lock, then finds the row already claimed and tries again. With N workers you have
serialised the claim path.

`SKIP LOCKED` is the fix:

```sql
SELECT id FROM transcode_chunks
WHERE state = 'pending'
ORDER BY job_id, chunk_index
LIMIT 1
FOR UPDATE SKIP LOCKED;
```

*"Lock a row I can have, and step over any row someone else is holding."* Worker B doesn't
block — it skips A's row and takes the next one. N workers, N different rows, no contention.

Available in MySQL 8.0+ and PostgreSQL 9.5+. This one clause is what makes a table a real
queue.

---

## Guarantee 2: give it back if the worker dies

The instinct is heartbeats and a crash handler. Both are wrong: a crashed process cannot run
its own cleanup, and a heartbeat protocol is a second distributed system guarding the first.

Invert it — a claim is a **lease that expires**:

```sql
UPDATE transcode_chunks
SET state = 'leased', lease_owner = ?, lease_expires_at = NOW() + INTERVAL 10 MINUTE
WHERE id = ?;
```

and the claim query treats expired leases as available:

```sql
WHERE state = 'pending'
   OR (state = 'leased' AND lease_expires_at < NOW())
```

**Recovery is the absence of a renewal.** No handler, no cleanup job, no lock service. A pod
that dies stops renewing, the lease lapses, another worker picks the row up. Nothing had to
notice the death.

A live worker renews while it works. Lease duration is a real trade-off: too short and slow
work gets stolen mid-flight; too long and recovery is slow. Rule of thumb — comfortably
longer than p99 item duration, and renew at half the interval.

---

## Guarantee 3: idempotency is what makes it safe

Leases mean an item can run **more than once**: a worker stalls past its lease, another takes
over, the first wakes and finishes. Both write output.

That is fine *if writes are idempotent*. Ours are, by construction:

```
movies/{movie_id}/360p/segment_00042.ts
```

The key is a pure function of `(movie_id, quality, chunk_index)`. Two workers processing the
same chunk write identical bytes to the identical key. The second overwrites the first and
nobody can tell.

Had we named outputs by attempt, or appended to a list, or incremented a counter, duplicate
execution would corrupt the result. **Idempotency is a naming decision made before the code
exists**, which is why it belongs in the design doc.

The database side gets the same treatment: `UNIQUE (job_id, chunk_index, quality)` makes a
duplicate row impossible rather than merely unlikely.

---

## When the table stops being enough

Honest limits, so this is a decision and not a preference:

| symptom | threshold | what to do |
|---|---|---|
| claim query shows in slow log | >100 claims/sec | index `(state, lease_expires_at)`, batch claims |
| workers idle waiting on claims | >500 claims/sec | Redis Streams |
| queue depth pressures the OLTP DB | millions of rows | separate queue store |
| you need fan-out to many consumers | any | this is a real broker's job — Kafka/Redpanda |

Our number: 540 items per film, each taking tens of seconds. That is **under one claim per
second**. We are three orders of magnitude from the first threshold.

Adding Kafka here would mean a JVM broker (or Redpanda), a schema for messages, a consumer
group, offset management, and a dead-letter path — to replace one SQL statement, while
*losing* the ability to claim a job in the same transaction that updates the job's row.

---

## The transactional bit people miss

With a broker, "mark chunk done" and "publish completion" are two systems. They can diverge:
DB commits, publish fails, and the item is silently lost — the failure that motivates the
transactional outbox pattern.

With rows as the queue there is nothing to diverge. Marking done *is* the state change:

```sql
BEGIN;
  UPDATE transcode_chunks SET state='done' WHERE id=? AND lease_owner=?;
  UPDATE upload_jobs SET updated_at=NOW() WHERE id=?;
COMMIT;
```

One transaction, atomic. The `AND lease_owner = ?` guard means a worker whose lease was
stolen cannot mark the row done — it lost the race and its write is rejected.

That guard is the whole concurrency-control story, in one clause.

---

## Related

- `docs/features/01-upload-pipeline.md` §5 — the concrete claim loop
- ADR-018 in `docs/MASTER.md`
- The `Queue` seam in MASTER §2 — this is the "channel → Redis Streams → Kafka" ladder's
  first rung, and swapping rungs should not touch calling code
