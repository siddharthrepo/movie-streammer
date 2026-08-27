CREATE TABLE IF NOT EXISTS upload_jobs (
    id            CHAR(36)     NOT NULL,
    movie_id      CHAR(36)     NOT NULL,
    source_key    VARCHAR(512) NOT NULL,
    source_size   BIGINT       DEFAULT NULL,
    s3_upload_id  VARCHAR(255) DEFAULT NULL,
    state         VARCHAR(32)  NOT NULL DEFAULT 'pending_upload',
    part_size     BIGINT       NOT NULL,
    part_count    INT          NOT NULL,
    chunk_count   INT          NOT NULL DEFAULT 0,
    error         TEXT,
    created_at    DATETIME(3)  NOT NULL,
    updated_at    DATETIME(3)  NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_upload_jobs_movie (movie_id),
    KEY idx_upload_jobs_state (state),
    KEY idx_upload_jobs_state_created (state, created_at),
    CONSTRAINT fk_upload_jobs_movie FOREIGN KEY (movie_id)
        REFERENCES movies (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
