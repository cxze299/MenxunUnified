ALTER TABLE study_weeks
  ADD COLUMN publication_status VARCHAR(16) NOT NULL DEFAULT 'published' AFTER outline_enabled;

ALTER TABLE study_weeks
  ADD COLUMN published_at DATETIME(3) NULL AFTER publication_status;

UPDATE study_weeks
SET publication_status='published', published_at=COALESCE(published_at, updated_at)
WHERE publication_status='published';

CREATE INDEX idx_group_publication_dates
  ON study_weeks (group_id, publication_status, start_date, end_date);
