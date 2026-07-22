CREATE TABLE IF NOT EXISTS seed_run_record_changes (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    seed_run_id BIGINT UNSIGNED NOT NULL,
    record_id INT UNSIGNED NOT NULL,
    change_type VARCHAR(32) NOT NULL,
    record_type VARCHAR(10) NULL,
    active_status VARCHAR(20) NULL,
    gender VARCHAR(10) NULL,
    action VARCHAR(10) NULL,
    action_date VARCHAR(20) NULL,
    deceased VARCHAR(3) NULL,
    display_name VARCHAR(500) NULL,
    date_of_birth VARCHAR(50) NULL,
    countries_json JSON NULL,
    INDEX idx_seed_run (seed_run_id),
    INDEX idx_seed_run_record (seed_run_id, record_id),
    CONSTRAINT fk_seed_run_record_changes_run FOREIGN KEY (seed_run_id) REFERENCES seed_runs(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
