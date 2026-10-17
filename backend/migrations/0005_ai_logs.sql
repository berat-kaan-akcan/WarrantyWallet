-- AI usage logging: receipt extraction and the fault assistant share one log table.
ALTER TABLE extraction_logs ADD COLUMN kind text NOT NULL DEFAULT 'receipt' CHECK (kind IN ('receipt', 'diagnosis'));
ALTER TABLE extraction_logs ADD COLUMN model text NOT NULL DEFAULT '';
CREATE INDEX extraction_logs_kind_idx ON extraction_logs (kind, created_at DESC);
