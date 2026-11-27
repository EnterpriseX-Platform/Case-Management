-- Scale: archive tables for closed cases, a daily summary for reports,
-- and indexes for large volumes.
CREATE TABLE IF NOT EXISTS cm_case_archive (LIKE cm_case INCLUDING ALL);
CREATE TABLE IF NOT EXISTS cm_event_archive (
  tenant_id text NOT NULL, seq bigint NOT NULL, case_id text NOT NULL, at timestamptz NOT NULL,
  type text NOT NULL, node text, data jsonb NOT NULL, PRIMARY KEY (tenant_id, seq)
);
CREATE INDEX IF NOT EXISTS cm_event_archive_case ON cm_event_archive (tenant_id, case_id, seq);
CREATE TABLE IF NOT EXISTS cm_work_archive (LIKE cm_work INCLUDING ALL);

CREATE TABLE IF NOT EXISTS cm_metric_daily (
  tenant_id   text NOT NULL,
  day         date NOT NULL,
  process_key text NOT NULL,
  started     int NOT NULL DEFAULT 0,
  completed   int NOT NULL DEFAULT 0,
  cancelled   int NOT NULL DEFAULT 0,
  cycle_hours double precision NOT NULL DEFAULT 0, -- sum, for averages
  PRIMARY KEY (tenant_id, day, process_key)
);

CREATE INDEX IF NOT EXISTS cm_case_status_created ON cm_case (tenant_id, status, created_at);
CREATE INDEX IF NOT EXISTS cm_case_process_created ON cm_case (tenant_id, process_key, created_at DESC);
