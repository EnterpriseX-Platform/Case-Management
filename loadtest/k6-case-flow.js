// k6 load test: each virtual user starts a case and completes its first step.
//   k6 run -e BASE=http://localhost:8080 -e PROCESS=purchase-request -e YEAR=2027 loadtest/k6-case-flow.js
// Development auth headers are used; behind OIDC pass -e TOKEN=<bearer>.
import http from "k6/http";
import { check } from "k6";

const BASE = __ENV.BASE || "http://localhost:8080";
const PROCESS = __ENV.PROCESS || "purchase-request";
const YEAR = Number(__ENV.YEAR || 2027);

export const options = {
  scenarios: {
    cases: { executor: "ramping-arrival-rate", startRate: 5, timeUnit: "1s", preAllocatedVUs: 50, maxVUs: 400,
      stages: [{ target: 50, duration: "1m" }, { target: 50, duration: "3m" }, { target: 0, duration: "30s" }] },
    inbox: { executor: "constant-vus", vus: 20, duration: "4m30s", exec: "inbox" },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
    "http_req_duration{name:inbox}": ["p(95)<300"],
    "http_req_duration{name:start}": ["p(95)<500"],
    "http_req_duration{name:complete}": ["p(95)<500"],
  },
};

function headers(user, roles) {
  const h = { "Content-Type": "application/json" };
  if (__ENV.TOKEN) h.Authorization = `Bearer ${__ENV.TOKEN}`;
  else { h["X-User"] = user; h["X-Roles"] = roles; }
  return { headers: h };
}

export default function () {
  const u = `load-${__VU}`;
  const req = headers(u, "requester");
  const c = http.post(`${BASE}/api/cases`, JSON.stringify({ processKey: PROCESS, fiscalYear: YEAR, title: `load ${__VU}-${__ITER}` }),
    { ...req, tags: { name: "start" } });
  if (!check(c, { "case created": (r) => r.status === 200 })) return;
  const caseId = c.json("id");
  const view = http.get(`${BASE}/api/cases/${caseId}`, { ...req, tags: { name: "case" } });
  const work = (view.json("work") || []).find((w) => w.status === "open");
  if (!work) return;
  const done = http.post(`${BASE}/api/work/${work.id}/complete`,
    JSON.stringify({ amount: 1000, reviewers: ["IT"] }), { ...req, tags: { name: "complete" } });
  check(done, { "step completed": (r) => r.status === 200 });
}

export function inbox() {
  const r = http.get(`${BASE}/api/work`, { ...headers("load-reviewer", "reviewer"), tags: { name: "inbox" } });
  check(r, { "inbox ok": (x) => x.status === 200 });
}
