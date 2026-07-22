CREATE TABLE IF NOT EXISTS seed_runs (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP NULL,
    json_source VARCHAR(500),
    status VARCHAR(20) NOT NULL DEFAULT 'running',
    duration_ms BIGINT UNSIGNED NULL,
    INDEX idx_started_at (started_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS seed_run_changes (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    seed_run_id BIGINT UNSIGNED NOT NULL,
    change_type VARCHAR(32) NOT NULL,
    entity VARCHAR(64) NOT NULL,
    record_type VARCHAR(10) NULL,
    count INT UNSIGNED NOT NULL DEFAULT 0,
    INDEX idx_seed_run (seed_run_id),
    CONSTRAINT fk_seed_run_changes_run FOREIGN KEY (seed_run_id) REFERENCES seed_runs(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
