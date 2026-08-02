-- Speeds up record-type filtering during FULLTEXT joins.
ALTER TABLE sanctions_records
    ADD INDEX idx_active_record_type (active_status, record_type);
