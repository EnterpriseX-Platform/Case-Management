-- Phase 0: optimistic locking, job executor, incidents, event log, subscriptions.

ALTER TABLE cm_case ADD COLUMN IF NOT EXISTS revision int NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS cm_job (
  tenant_id   text NOT NULL DEFAULT 'default',
  id          text NOT NULL,
  case_id     text NOT NULL,
  instance_id text NOT NULL,
  token_id    text NOT NULL,
  status      text NOT NULL,
  next_run_at timestamptz NOT NULL,
  lease_until timestamptz,
  data        jsonb NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS cm_job_due   ON cm_job (status, next_run_at);
CREATE INDEX IF NOT EXISTS cm_job_token ON cm_job (tenant_id, instance_id, token_id);

CREATE TABLE IF NOT EXISTS cm_incident (
  tenant_id  text NOT NULL DEFAULT 'default',
  id         text NOT NULL,
  case_id    text NOT NULL,
  status     text NOT NULL,
  created_at timestamptz NOT NULL,
  data       jsonb NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS cm_incident_open ON cm_incident (tenant_id, status, created_at DESC);

-- append-only; partition by month in large installations
CREATE TABLE IF NOT EXISTS cm_event (
  tenant_id text NOT NULL DEFAULT 'default',
  seq       bigserial,
  case_id   text NOT NULL,
  at        timestamptz NOT NULL,
  type      text NOT NULL,
  node      text,
  data      jsonb NOT NULL,
  PRIMARY KEY (tenant_id, seq)
);
CREATE INDEX IF NOT EXISTS cm_event_case ON cm_event (tenant_id, case_id, seq);
CREATE INDEX IF NOT EXISTS cm_event_time ON cm_event (tenant_id, at);

CREATE TABLE IF NOT EXISTS cm_subscription (
  tenant_id text NOT NULL DEFAULT 'default',
  case_id   text NOT NULL,
  token_id  text NOT NULL,
  kind      text NOT NULL,
  name      text NOT NULL,
  PRIMARY KEY (tenant_id, case_id, token_id, kind, name)
);
CREATE INDEX IF NOT EXISTS cm_subscription_name ON cm_subscription (tenant_id, kind, name);
