# Services

One backend image runs as several services, chosen with `MODE` (comma separated).
Each service scales on its own. All of them share one Postgres database.

| Service | `MODE` | Does | Scale by | Replicas |
|---|---|---|---|---|
| web | (frontend image) | User interface | users | 2+ |
| api | `api` | HTTP API: cases, work, studio | users | 2+ (autoscaled on CPU) |
| engine | `engine` | In-process jobs: decision tables | case volume | 2+ (autoscaled on CPU) |
| integration | `integration` | Calls to other systems (connectors, rules) | number/slowness of remote calls | 2+ |
| scheduler | `scheduler` | Timers, SLA goals/deadlines, escalations | — | 2 (one active, one standby) |
| notify | `notify` | E-mail delivery from the outbox, with retries | e-mail volume | 1+ |

`MODE=worker` runs engine, integration, scheduler and notify together, and
`MODE=all` (the default) runs everything in one process, which is handy for development.

## How work moves between services

- **Jobs** carry a queue (`engine` or `integration`). A worker only claims jobs of
  the queues it serves, using `FOR UPDATE SKIP LOCKED` with a lease, so any number
  of replicas share a queue. A slow remote system fills the integration queue but
  never holds up decision tables.
- **The scheduler** takes a Postgres advisory lock before it runs. Standby replicas
  retry every few seconds. If the active one dies, Postgres frees the lock and a
  standby takes over (about a second in the compose test).
- **E-mail** is written to an outbox in the same transaction as the case change.
  The notify service sends it, and retries with backoff up to 8 times before
  marking it failed. Without `SMTP_HOST`, messages are dropped; in-app
  notifications still work.
- **Schema migrations** run under an advisory lock and are recorded in
  `cm_migration`. Replicas that start later run no DDL at all, so they never
  wait on tables that other replicas are using.

## Running

```bash
docker compose up -d --scale engine=3 --scale integration=2
```

On Kubernetes, apply `deploy/k8s/case-management.yaml` once per environment. The
Jenkinsfile then rolls new image tags to every deployment.

Every background service serves `/healthz` and `/metrics` on port 8081.
