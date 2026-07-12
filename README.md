# movie_streamer

A production-grade movie streaming platform built as a learning/portfolio project:
upload large videos, transcode them into HLS adaptive-bitrate renditions with a
distributed chunked pipeline, and stream them to players.

**Design docs live in [`docs/`](./docs/) — read [`docs/MASTER.md`](./docs/MASTER.md) first.**
Every architecture decision is recorded in [`docs/DECISIONS.md`](./docs/DECISIONS.md).

## Stack (local-first)

- **Go** services (`cmd/`), shared libs in `internal/`
- **Postgres** — metadata + job state
- **MinIO** — S3-compatible object storage (raw + HLS output)
- **RabbitMQ** — job broker (competing consumers)
- **ffmpeg** — transcoding (invoked by the transcode-worker)

## Quick start

```sh
cp .env.example .env      # adjust if needed
make up                   # start Postgres + MinIO + RabbitMQ
make run-upload           # run the upload-service
curl localhost:8080/healthz
make down                 # stop infra
```

Consoles: MinIO → http://localhost:9001, RabbitMQ → http://localhost:15672.

## Status

Milestone 1 (upload skeleton) in progress. See the milestone roadmap in
[`docs/MASTER.md`](./docs/MASTER.md).
