-- Service split: job queues per worker kind, and an outbox for e-mail.
ALTER TABLE cm_job ADD COLUMN IF NOT EXISTS queue text NOT NULL DEFAULT 'integration';
CREATE INDEX IF NOT EXISTS cm_job_queue_due ON cm_job (tenant_id, queue, next_run_at);

CREATE TABLE IF NOT EXISTS cm_outbox (
  tenant_id   text NOT NULL DEFAULT 'default',
  id          text NOT NULL,
  status      text NOT NULL,
  next_run_at timestamptz NOT NULL,
  lease_until timestamptz,
  data        jsonb NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS cm_outbox_due ON cm_outbox (tenant_id, status, next_run_at);
