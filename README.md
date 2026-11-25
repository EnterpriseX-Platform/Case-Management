# Case Management

Low-code case management with a BPMN 2.0 engine. Backend in Go, frontend in Next.js.

- `backend/`: BPMN parser + token engine, versioning (one published version per fiscal year, maker-checker), screen bindings, migration, import (BPMN / pack / JSON / CSV / XLSX), REST API, Postgres or in-memory store
- `frontend/`: screens for every role: worklist, task screen, cases, flow designer (bpmn-js), form builder, screen builder, step-to-screen mapping, supervisor dashboard, import, migration, audit
- `packs/purchase-request/`: sample pack (purchase request + urgent variant), rebuild with `python3 build.py`

```bash
cd backend && SEED_PACK=../packs/purchase-request.cmpack go run ./cmd/server
cd frontend && npm install && npm run dev
```
