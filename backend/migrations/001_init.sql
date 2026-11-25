-- Case Management core schema. Every row carries tenant_id for multi-tenancy;
-- the full object lives in `data` (jsonb), hot columns are duplicated for indexing.

CREATE TABLE IF NOT EXISTS cm_process (
  tenant_id  text NOT NULL DEFAULT 'default',
  key        text NOT NULL,
  data       jsonb NOT NULL,
  PRIMARY KEY (tenant_id, key)
);

CREATE TABLE IF NOT EXISTS cm_version (
  tenant_id   text NOT NULL DEFAULT 'default',
  id          text NOT NULL,
  process_key text NOT NULL,
  no          int  NOT NULL,
  status      text NOT NULL,
  fiscal_year int,
  checksum    text NOT NULL,
  data        jsonb NOT NULL,
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, process_key, no)
);
-- one published version per case type and fiscal year
CREATE UNIQUE INDEX IF NOT EXISTS cm_version_published
  ON cm_version (tenant_id, process_key, fiscal_year) WHERE status = 'published';

CREATE TABLE IF NOT EXISTS cm_binding (
  tenant_id  text NOT NULL DEFAULT 'default',
  version_id text NOT NULL,
  data       jsonb NOT NULL,
  PRIMARY KEY (tenant_id, version_id)
);

CREATE TABLE IF NOT EXISTS cm_form (
  tenant_id text NOT NULL DEFAULT 'default',
  key       text NOT NULL,
  data      jsonb NOT NULL,
  PRIMARY KEY (tenant_id, key)
);

CREATE TABLE IF NOT EXISTS cm_screen (
  tenant_id text NOT NULL DEFAULT 'default',
  key       text NOT NULL,
  data      jsonb NOT NULL,
  PRIMARY KEY (tenant_id, key)
);

CREATE TABLE IF NOT EXISTS cm_case (
  tenant_id   text NOT NULL DEFAULT 'default',
  id          text NOT NULL,
  number      text NOT NULL,
  process_key text NOT NULL,
  version_id  text NOT NULL,
  fiscal_year int,
  parent_id   text,
  status      text NOT NULL,
  created_at  timestamptz NOT NULL,
  data        jsonb NOT NULL,
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, number)
);
CREATE INDEX IF NOT EXISTS cm_case_version ON cm_case (tenant_id, version_id, status);
CREATE INDEX IF NOT EXISTS cm_case_parent  ON cm_case (tenant_id, parent_id);

CREATE TABLE IF NOT EXISTS cm_case_seq (
  tenant_id text NOT NULL DEFAULT 'default',
  prefix    text NOT NULL,
  value     int  NOT NULL,
  PRIMARY KEY (tenant_id, prefix)
);

CREATE TABLE IF NOT EXISTS cm_work (
  tenant_id  text NOT NULL DEFAULT 'default',
  id         text NOT NULL,
  case_id    text NOT NULL,
  role       text,
  assignee   text,
  status     text NOT NULL,
  due_at     timestamptz,
  created_at timestamptz NOT NULL,
  data       jsonb NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS cm_work_inbox ON cm_work (tenant_id, status, role, assignee);
CREATE INDEX IF NOT EXISTS cm_work_case  ON cm_work (tenant_id, case_id);

CREATE TABLE IF NOT EXISTS cm_timer (
  tenant_id   text NOT NULL DEFAULT 'default',
  id          text NOT NULL,
  instance_id text NOT NULL,
  token_id    text NOT NULL,
  due_at      timestamptz NOT NULL,
  data        jsonb NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS cm_timer_due ON cm_timer (due_at);

CREATE TABLE IF NOT EXISTS cm_audit (
  tenant_id text NOT NULL DEFAULT 'default',
  seq       bigserial,
  at        timestamptz NOT NULL,
  ref       text NOT NULL,
  data      jsonb NOT NULL,
  PRIMARY KEY (tenant_id, seq)
);
CREATE INDEX IF NOT EXISTS cm_audit_ref ON cm_audit (tenant_id, ref, seq DESC);

CREATE TABLE IF NOT EXISTS cm_import (
  tenant_id  text NOT NULL DEFAULT 'default',
  id         text NOT NULL,
  created_at timestamptz NOT NULL,
  data       jsonb NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
