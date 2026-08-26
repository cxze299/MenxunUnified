ALTER TABLE registration_requests
  ADD COLUMN applicant_user_id BIGINT UNSIGNED NULL AFTER group_id;

ALTER TABLE registration_requests
  ADD COLUMN request_kind VARCHAR(24) NOT NULL DEFAULT 'new_account' AFTER applicant_user_id;

ALTER TABLE registration_requests
  ADD COLUMN terms_accepted_at DATETIME(3) NULL AFTER reviewed_at;

CREATE INDEX idx_registration_applicant_status
  ON registration_requests (applicant_user_id, status, created_at);

CREATE TABLE IF NOT EXISTS reflections (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  group_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  checkin_record_id BIGINT UNSIGNED NULL,
  logical_date DATE NOT NULL,
  content TEXT NOT NULL,
  visibility VARCHAR(16) NOT NULL DEFAULT 'private',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  deleted_at DATETIME(3) NULL,
  KEY idx_reflections_group_date (group_id, logical_date),
  KEY idx_reflections_user_date (user_id, logical_date),
  KEY idx_reflections_checkin (checkin_record_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS migration_batches (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  source_site VARCHAR(128) NOT NULL,
  group_id BIGINT UNSIGNED NOT NULL,
  mode VARCHAR(16) NOT NULL,
  status VARCHAR(24) NOT NULL,
  report_path VARCHAR(1024) NOT NULL DEFAULT '',
  summary_json JSON NULL,
  started_by BIGINT UNSIGNED NULL,
  started_at DATETIME(3) NOT NULL,
  finished_at DATETIME(3) NULL,
  KEY idx_migration_group_started (group_id, started_at),
  KEY idx_migration_source_started (source_site, started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS migration_source_records (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  batch_id BIGINT UNSIGNED NOT NULL,
  source_site VARCHAR(128) NOT NULL,
  source_record_key VARCHAR(255) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id BIGINT UNSIGNED NULL,
  status VARCHAR(24) NOT NULL,
  message VARCHAR(1024) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_migration_source_record (source_site, source_record_key),
  KEY idx_migration_batch_status (batch_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_assets_checksum_size
  ON assets (checksum_sha256, file_size);
