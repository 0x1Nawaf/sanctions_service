CREATE TABLE IF NOT EXISTS custom_lists (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE sanctions_records
    MODIFY COLUMN id INT UNSIGNED NOT NULL AUTO_INCREMENT;

ALTER TABLE sanctions_records
    ADD COLUMN custom_list_id INT UNSIGNED NULL DEFAULT NULL AFTER profile_notes,
    ADD INDEX idx_custom_list_id (custom_list_id),
    ADD CONSTRAINT fk_custom_list FOREIGN KEY (custom_list_id) REFERENCES custom_lists(id) ON DELETE CASCADE;
