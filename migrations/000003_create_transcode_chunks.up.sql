CREATE TABLE IF NOT EXISTS transcode_chunks (
    id                BIGINT       NOT NULL AUTO_INCREMENT,
    job_id            CHAR(36)     NOT NULL,
    movie_id          CHAR(36)     NOT NULL,
    chunk_index       INT          NOT NULL,
    quality           VARCHAR(8)   NOT NULL,
    start_ms          BIGINT       NOT NULL,
    end_ms            BIGINT       NOT NULL,
    segment_offset    INT          NOT NULL,
    state             VARCHAR(16)  NOT NULL DEFAULT 'pending',
    lease_owner       VARCHAR(64)  DEFAULT NULL,
    lease_expires_at  DATETIME(3)  DEFAULT NULL,
    attempts          INT          NOT NULL DEFAULT 0,
    error             TEXT,
    created_at        DATETIME(3)  NOT NULL,
    updated_at        DATETIME(3)  NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_chunk_job_index_quality (job_id, chunk_index, quality),
    KEY idx_chunk_claim (state, lease_expires_at),
    KEY idx_chunk_job_state (job_id, state),
    CONSTRAINT fk_chunks_job FOREIGN KEY (job_id)
        REFERENCES upload_jobs (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
