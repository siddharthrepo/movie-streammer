CREATE TABLE IF NOT EXISTS movies (
    id          CHAR(36)     NOT NULL,
    title       VARCHAR(255) NOT NULL,
    description TEXT,
    duration_ms BIGINT       DEFAULT NULL,
    status      VARCHAR(32)  NOT NULL DEFAULT 'draft',
    created_at  DATETIME(3)  NOT NULL,
    updated_at  DATETIME(3)  NOT NULL,
    PRIMARY KEY (id),
    KEY idx_movies_status (status),
    KEY idx_movies_title (title),
    KEY idx_movies_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
