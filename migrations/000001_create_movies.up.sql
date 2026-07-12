CREATE TABLE movies (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    filename      text        NOT NULL,
    object_key    text        NOT NULL UNIQUE,
    size_bytes    bigint      NOT NULL,
    content_type  text        NOT NULL,
    status        text        NOT NULL DEFAULT 'pending_upload'
                  CHECK (status IN ('pending_upload', 'uploaded', 'aborted')),
    upload_id     text        NOT NULL DEFAULT '',
    part_size     bigint      NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
