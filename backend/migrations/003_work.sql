-- Phase 3: work management.
CREATE TABLE IF NOT EXISTS cm_doc (
  tenant_id text NOT NULL DEFAULT 'default',
  kind      text NOT NULL,   -- user | queue | settings
  key       text NOT NULL,
  data      jsonb NOT NULL,
  PRIMARY KEY (tenant_id, kind, key)
);

CREATE TABLE IF NOT EXISTS cm_notification (
  tenant_id text NOT NULL DEFAULT 'default',
  id        text NOT NULL,
  usr       text NOT NULL,
  at        timestamptz NOT NULL,
  read      boolean NOT NULL DEFAULT false,
  data      jsonb NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS cm_notification_user ON cm_notification (tenant_id, usr, at DESC);

ALTER TABLE cm_work ADD COLUMN IF NOT EXISTS goal_at timestamptz;
ALTER TABLE cm_work ADD COLUMN IF NOT EXISTS sla_open boolean NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS cm_work_sla ON cm_work (tenant_id, sla_open) WHERE sla_open;
