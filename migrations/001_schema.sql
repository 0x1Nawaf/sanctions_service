-- Reference tables

CREATE TABLE IF NOT EXISTS sanctions_ref_countries (
    code VARCHAR(20) PRIMARY KEY,
    name VARCHAR(100),
    is_territory TINYINT(1) NOT NULL DEFAULT 0,
    profile_url VARCHAR(255),
    INDEX idx_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_ref_occupations (
    code SMALLINT UNSIGNED PRIMARY KEY,
    name VARCHAR(200),
    INDEX idx_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_ref_relationships (
    code SMALLINT UNSIGNED PRIMARY KEY,
    name VARCHAR(100),
    INDEX idx_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_ref_sanctions_lists (
    id INT UNSIGNED PRIMARY KEY,
    name VARCHAR(500),
    status VARCHAR(20),
    description2_id SMALLINT UNSIGNED,
    INDEX idx_name (name(191))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_ref_description1 (
    description1_id SMALLINT UNSIGNED PRIMARY KEY,
    record_type VARCHAR(10),
    name VARCHAR(200),
    INDEX idx_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_ref_description2 (
    description2_id SMALLINT UNSIGNED PRIMARY KEY,
    description1_id SMALLINT UNSIGNED,
    name VARCHAR(200),
    INDEX idx_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_ref_description3 (
    description3_id SMALLINT UNSIGNED PRIMARY KEY,
    description2_id SMALLINT UNSIGNED,
    name VARCHAR(200),
    INDEX idx_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Main records

CREATE TABLE IF NOT EXISTS sanctions_records (
    id INT UNSIGNED PRIMARY KEY,
    record_type VARCHAR(10),
    action VARCHAR(10),
    action_date VARCHAR(20),
    gender VARCHAR(10),
    active_status VARCHAR(20),
    deceased VARCHAR(3),
    profile_notes LONGTEXT,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_record_type (record_type),
    INDEX idx_active_status (active_status),
    INDEX idx_active_id (active_status, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Child tables

CREATE TABLE IF NOT EXISTS sanctions_names (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    name_type VARCHAR(50),
    title_honorific VARCHAR(100),
    first_name VARCHAR(200),
    middle_name VARCHAR(200),
    surname VARCHAR(200),
    maiden_name VARCHAR(200),
    suffix VARCHAR(500),
    single_string_name VARCHAR(500),
    original_script_name VARCHAR(500),
    entity_name VARCHAR(500),
    INDEX idx_record_id (record_id),
    INDEX idx_primary_lookup (record_id, name_type, id),
    INDEX idx_name_type_record (name_type, record_id),
    FULLTEXT INDEX sanctions_names_fulltext (first_name, surname, single_string_name, entity_name),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_descriptions (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    description1_id SMALLINT UNSIGNED,
    description2_id SMALLINT UNSIGNED,
    description3_id SMALLINT UNSIGNED,
    INDEX idx_record_id (record_id),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_roles (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    role_type VARCHAR(50),
    occ_cat_code SMALLINT UNSIGNED,
    title VARCHAR(500),
    since_day VARCHAR(2),
    since_month VARCHAR(3),
    since_year VARCHAR(4),
    to_day VARCHAR(2),
    to_month VARCHAR(3),
    to_year VARCHAR(4),
    INDEX idx_record_id (record_id),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_dates (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    date_type VARCHAR(50),
    day VARCHAR(2),
    month VARCHAR(3),
    year VARCHAR(4),
    note TEXT,
    INDEX idx_record_id (record_id),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_birth_places (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    place VARCHAR(500),
    INDEX idx_record_id (record_id),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_refs (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    sanctions_ref_id INT UNSIGNED,
    since_day VARCHAR(2),
    since_month VARCHAR(3),
    since_year VARCHAR(4),
    to_day VARCHAR(2),
    to_month VARCHAR(3),
    to_year VARCHAR(4),
    INDEX idx_record_id (record_id),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_countries (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    country_type VARCHAR(50),
    country_code VARCHAR(20),
    INDEX idx_record_id (record_id),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_id_numbers (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    id_type VARCHAR(100),
    id_value VARCHAR(500),
    id_notes TEXT,
    INDEX idx_record_id (record_id),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_sources (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    url TEXT,
    INDEX idx_record_id (record_id),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_images (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    url TEXT,
    INDEX idx_record_id (record_id),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_addresses (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    address_line VARCHAR(500),
    address_city VARCHAR(200),
    address_country VARCHAR(100),
    url TEXT,
    INDEX idx_record_id (record_id),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sanctions_associations (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    record_id INT UNSIGNED NOT NULL,
    associate_id INT UNSIGNED NOT NULL,
    relationship_code SMALLINT UNSIGNED,
    is_ex TINYINT(1) NOT NULL DEFAULT 0,
    INDEX idx_record_id (record_id),
    INDEX idx_associate_id (associate_id),
    UNIQUE KEY sanctions_assoc_unique (record_id, associate_id, relationship_code),
    FOREIGN KEY (record_id) REFERENCES sanctions_records(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
