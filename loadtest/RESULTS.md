# Load test results

`k6-case-flow.js`, shortened run (15 s ramp + 30 s at 50 new cases/s, plus 20 VUs reading the inbox).
Single API process (MODE=all) and PostgreSQL 16 on the same laptop (Apple M1, 16 GB).

| Date | Commit | Requests | Errors | start p95 | complete p95 | inbox p95 | Notes |
|---|---|---|---|---|---|---|---|
| 2026-11-26 | b8e397d+ | 26,292 | 0 % | 1.34 s ✗ | 512 ms ✗ | 239 ms | inbox returned every open item |
| 2026-11-26 | this commit | 34,196 | 0 % | 401 ms ✓ | 179 ms ✓ | 104 ms ✓ | inbox paged (200, most urgent first) + queue indexes |

Engine-only benchmark (`go test ./internal/engine -bench Steps`): ~228,000 steps/s.
End-to-end on Postgres (`TestParallelCasesThroughput`, 200 parallel cases): ~950 steps/s.
