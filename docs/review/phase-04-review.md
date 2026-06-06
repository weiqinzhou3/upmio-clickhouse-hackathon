# Phase 04 Review Report

- Version: 1.0
- Date: 2026-06-06
- Reviewer: Claude (Opus 4.7)
- Owner: 周钦伟 (zqw)
- Scope: Review of Codex's Phase 04 implementation against
  `docs/phases/phase-04-healthcheck.md` v0.5
- Verdict: **PASS — proceed to Phase 05** (four non-blocking findings in §7)

## 1. Phase 04 Context

Phase 04 implements Day1 healthcheck and a structured acceptance report for
UPM-managed ClickHouse clusters through the existing `upm-api-server`. The
healthcheck validates Kubernetes resources, Keeper quorum, ClickHouse SQL
reachability, `system.clusters` topology, replicated table health, a reserved
Distributed write/read probe, metrics endpoint availability, and response
safety — all through the existing NodePort service.

Out of scope (per spec §3):

- Backup/restore validation, Prometheus/Grafana dashboard validation.
- Prometheus query summary API (Phase 05).
- Automatic remediation, synthetic workload, user business schema management.
- GrpcCall dependency, new UPMIO CRDs.
- Persistent report database or long-term operation history.

Phase 04 v0.5 (2026-06-06) aligns with the existing Phase 03 `upm-api-server`
deployment, the 2x2+3 lab topology, in-memory latest-report cache, NodePort
validation, metrics WARN severity policy, and validation-object drift rules.

## 2. Review Method

1. Re-read `docs/phases/phase-04-healthcheck.md` v0.5 plus linked design docs.
2. Inspect every file changed in Phase 04 commits:
   - `api-server/internal/api/server.go` (+33 lines, healthcheck routes + cache)
   - `api-server/internal/api/server_test.go` (+49 lines, 3 new tests)
   - `api-server/internal/model/healthcheck.go` (+102 lines, report model)
   - `api-server/internal/model/healthcheck_test.go` (+52 lines, 3 tests)
   - `api-server/internal/kube/store.go` (+677 lines, healthcheck implementation)
   - `api-server/internal/platform/store.go` (+1 line, interface)
   - `api-server/internal/clickhouse/client.go` (unchanged, still unused)
   - `api-server/go.mod` / `go.sum` (+3 deps for SPDY exec)
   - `docs/api/upm-api-server-v1.md` (+175 lines, §6.5–§6.6, §7 updated)
   - `docs/design/data-architecture.md` (+8 lines, in-memory cache decision)
   - `clickhouse/phase-04/README.md`, `runtime-validation.md`,
     `scripts/validate-healthcheck-runtime.sh`, report JSONs
   - `clickhouse/phase-03/manifests/upm-api-server.yaml` (no diff — RBAC unchanged)
3. Re-run quality gates on the review host:
   - `gofmt -l .` (clean)
   - `go vet ./...` (clean)
   - `go test -race -count=1 ./...` (all PASS)
   - `go build ./...` (clean)
   - `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/upm-api-server`
     (ELF 64-bit, statically linked)
   - `bash -n` on `validate-healthcheck-runtime.sh` (OK)
4. Cross-check runtime claims in
   `clickhouse/phase-04/runtime-validation.md` against the script that
   produces them.

The review host has no live lab-cluster access today, so the runtime PASS
evidence is read from saved reports and the validation script. The script was
inspected line by line.

## 3. Acceptance Criteria Verification

Phase 04 spec §9 lists 16 criteria. Verified line by line.

| # | Criterion | Status | Evidence |
|---|---|---|---|
| 1 | `POST /healthcheck` returns a structured report with every required check name | PASS | `store.go:378-408` runs all 14 checks in sequence; `runtime-healthcheck-report.json` contains all 14 check names listed in §6; runtime script asserts each by name |
| 2 | Report status is `PASS` only when all critical checks pass and no warnings exist | PASS | `model/healthcheck.go:70-101` (`Finalize`): FAIL on critical → overall FAIL; WARN on any → overall WARN (if not already FAIL); no checks → SKIPPED. `TestHealthcheckReportAggregation` and `TestHealthcheckCriticalFailureWins` cover the key paths; runtime report shows `PASS` with `summary.warnings=0` |
| 3 | `GET /healthcheck/latest` returns latest in-memory report or structured `404 HEALTHCHECK_REPORT_NOT_FOUND` | PASS | `server.go:189-210`: reads from `s.latest` map under RLock, returns 404 with `HEALTHCHECK_REPORT_NOT_FOUND` code and namespace/name details when absent. `TestRunHealthcheckStoresLatestReport` and `TestGetLatestHealthcheckReturnsNotFoundBeforeRun` cover both paths |
| 4 | Keeper `ruok` and `mntr` evidence is included or summarized | PASS | `store.go:482-530` (`checkKeeperRUOK`): execs bash `/dev/tcp` ruok probe into each Keeper pod, returns per-pod `results` and `failures` maps. `checkKeeperRoles` (line 532–557): execs `mntr`, parses `zk_server_state`, returns `roles`/`leaders`/`failures` per pod |
| 5 | `SELECT 1` evidence is included | PASS | `store.go:559-578` (`checkClickHouseSelect`): runs `SELECT 1 FORMAT TSV` on the first server pod, includes pod name and `result: "1"` in evidence |
| 6 | `system.clusters` evidence matches expected topology dynamically | PASS | `store.go:580-622` (`checkSystemClusters`): queries `system.clusters WHERE cluster='upm_cluster'`, parses TSV, counts shards and replicas-per-shard, compares against `cluster.Topology` from the live cluster summary. Evidence includes `expectedShards`/`expectedReplicas` and actual counts. Runtime report confirms 2 shards × 4 total rows for the 2×2 topology |
| 7 | `system.replicas` evidence shows no readonly/session-expired state | PASS | `store.go:703-751` (`checkReplicaHealth`): queries `clusterAllReplicas` on `system.replicas` for the healthcheck table, checks `is_readonly=0`, `is_session_expired=0`, `total_replicas=ReplicasPerShard`, `active_replicas=ReplicasPerShard`, `queue_size=0`. Any deviation → FAIL with per-replica `unhealthyReplicas` detail |
| 8 | Write/read probe is idempotent and isolated from business data | PASS | `store.go:624-701` (`checkWriteReadProbe`): uses reserved `upm_healthcheck` database; DDL is `IF NOT EXISTS`; truncates before insert; inserts 8 deterministic rows; syncs replicas on all servers; asserts row count == 8, shard count == topology.Shards, replica distribution == 4 each; validates table definitions against drift. Runtime script independently cross-checks `SELECT count()` and shard/replica assertions via kubectl exec |
| 9 | Healthcheck does not require ClickHouse `GrpcCall` | PASS | All ClickHouse communication is SQL via `service-ctl.sh login --query` or `--multiquery` over pod exec. No gRPC import, no gRPC dial, no gRPC proto |
| 10 | Metrics endpoint is checked and reported as warning severity | PASS | `store.go:753-772` (`checkMetricsEndpoint`) executes a curl request to the local metrics endpoint and reads five lines. It checks for a `#` prefix. Severity is `HealthSeverityWarning`; a failure reports `WARN` status. Runtime report shows `metrics_endpoint` as `PASS` |
| 11 | Healthcheck report redacts secrets | PASS | `store.go:779-801` (`addSecretLeakageCheck`): marshals the completed report, scans lowercase for `clickhouse_admin_password`, `aes_secret_key`, `secretkeyref`, reports matched patterns. `TestHealthcheckReportDoesNotRequireSecretFields` covers this at model level. Runtime script greps saved report JSONs — no matches |
| 12 | Unit tests cover report status aggregation logic | PASS | `model/healthcheck_test.go`: `TestHealthcheckReportAggregation` (1 PASS + 1 WARN → WARN), `TestHealthcheckCriticalFailureWins` (1 WARN + 1 critical FAIL → FAIL). Both pass with `-race` |
| 13 | Unit tests cover latest-report cache behavior | PASS | `api/server_test.go`: `TestRunHealthcheckStoresLatestReport` (POST → GET returns stored report), `TestGetLatestHealthcheckReturnsNotFoundBeforeRun` (GET before POST → 404). Both pass with `-race` |
| 14 | Unit tests cover secret leakage prevention in reports | PASS | `model/healthcheck_test.go`: `TestHealthcheckReportDoesNotRequireSecretFields` marshals a report with `no_secret_leakage` check and asserts absence of three secret-like tokens. `api/server_test.go`: `TestCreateClusterDoesNotEchoSecretMaterial` and `TestCreateClusterDoesNotLogRequestSecretMaterial` (carried forward from Phase 03). All pass with `-race` |
| 15 | Failure cases return structured error responses | PASS | `server.go:171-179`: `runHealthcheck` calls `s.store.RunHealthcheck` which can return errors from `GetCluster` (e.g. `CLUSTER_NOT_FOUND`) or internal errors — all flow through `writeError` which produces structured `{code, message, details, requestId}`. `server.go:200-207`: `getLatestHealthcheck` produces structured `HEALTHCHECK_REPORT_NOT_FOUND` with namespace/name details |
| 16 | Runtime validation calls real API over NodePort and verifies report against live K8s/ClickHouse state | PASS | `runtime-validation.md` records `PASS phase04_healthcheck_runtime_validation` (2026-06-06). Script validates: (a) all 14 checks present and PASS, (b) metrics_endpoint PASS or WARN, (c) no secret leakage in saved JSONs, (d) latest report cache returns same `startedAt`, (e) independent `kubectl exec` cross-checks: `dist_events` row count == 8, shard distribution == 2, replica health from `clusterAllReplicas` |

All 16 criteria are met.

## 4. Quality Gates — Re-executed in This Review

```text
gofmt -l .                                                          (clean)
go vet ./...                                                        (clean)
go test -race -count=1 ./...                                        (all PASS)
go build ./...                                                      (clean)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/upm-api-server (ELF/static)
bash -n clickhouse/phase-04/scripts/validate-healthcheck-runtime.sh (OK)
```

Spot inspection notes from the rendered code:

- **Healthcheck routes** (`server.go:45-46`) use Go 1.22+ method-aware path
  routing: `POST /clusters/{namespace}/{name}/healthcheck` and
  `GET /clusters/{namespace}/{name}/healthcheck/latest`. Unregistered methods
  get 405 without bespoke code.
- **In-memory cache** (`server.go:28-29, 182-184, 196-198`): `sync.RWMutex`-
  guarded `map[string]HealthcheckReport` keyed by `namespace/name`. Correctly
  uses `RLock` for reads and `Lock` for writes.
- **Pod exec** (`store.go:822-860`): SPDY-based `remotecommand.NewSPDYExecutor`
  with `StreamWithContext`, respecting context cancellation. Uses bash
  `/dev/tcp` for Keeper four-letter-command probes — avoids the missing `nc`
  binary issue documented in `runtime-validation.md §Notes`.
- **Validation DDL** (`store.go:862-884`): uses `{CREATE DATABASE, CREATE TABLE}
  IF NOT EXISTS ON CLUSTER`, deterministic table names (`upm_healthcheck.
  local_events` / `dist_events`), `TRUNCATE` before insert, and post-probe
  `SHOW CREATE TABLE` drift checks. Correctly uses `insert_distributed_sync=1`
  to avoid async replication races.
- **Topology is read dynamically** (`store.go:368`): `GetCluster` retrieves the
  live `ClusterSummary` with `Topology` from Phase 03; every check uses
  `cluster.Topology.Shards`, `.ReplicasPerShard`, `.KeeperReplicas` — never
  hardcoded.
- **Cross-shard assertions** (`store.go:665-678`): `throwIf` SQL guards verify
  row count (8), shard coverage (all shards), and replica distribution (4 per
  replica). Each assertion returns its own failure evidence with pod/assertion/
  stderr.
- **Evidence truncation** (`store.go:1034-1039`): `trimEvidence` caps error
  output at 1024 chars — prevents unbounded evidence fields from oversized
  ClickHouse error strings.
- **`resourceMatches` reuse**: pod/PVC/Service/Endpoint filtering all use the
  same label-based matching from Phase 03 — no parallel discovery path.
- **`internal/clickhouse`**: package still compiles but is not imported by
  anything. The healthcheck bypasses the HTTP client entirely, using pod exec
  and `service-ctl.sh login` instead. See finding F1.

## 5. Runtime Evidence Review

`runtime-validation.md` (2026-06-06, PASS) lines up with the script that
produced it:

1. **Prerequisites.** Waits for Keeper UnitSet (3/3 ready), Server UnitSet
   (4/4 ready), and API server rollout before proceeding. Hard exit on timeout.
2. **Healthcheck execution.** POSTs to the real NodePort, saves report JSON,
   asserts `status` is `PASS` or `WARN`, asserts `summary.failed == 0`.
3. **All 14 checks.** Iterates each critical check name, asserts `.status ==
   "PASS"` via `jq -e`. The `metrics_endpoint` check is asserted as `PASS` or
   `WARN` separately.
4. **Secret scan.** Greps saved JSON for `CLICKHOUSE_ADMIN_PASSWORD`,
   `AES_SECRET_KEY`, `secretKeyRef` — exit non-zero on any hit.
5. **Latest cache.** GETs `/latest`, asserts `startedAt` matches the POST
   report exactly, and `status` matches.
6. **Database cross-check.** Independent `kubectl exec`:
   - `SELECT count() FROM upm_healthcheck.dist_events` → asserts `8`.
   - Assert shard coverage: `SELECT _shard_num ... GROUP BY` → count must be 2.
   - Assert replica health: `clusterAllReplicas` → every replica must have
     `total_replicas=2`, `active_replicas=2`, `is_readonly=0`, `queue_size=0`.

The chain "API POST → report PASS → kubectl exec cross-check → latest cache
match" is reproduced end-to-end in a script with hard exits. The saved
`runtime-healthcheck-report.json` confirms all 14 checks PASS with topology
evidence matching the 2×2+3 cluster.

The one real-execution-only fix documented in `runtime-validation.md §Notes` —
bash `/dev/tcp` for Keeper probes instead of `nc` — is visible in code at
`store.go:493` and `store.go:540`.

## 6. Scope and Decision Boundaries — Held Cleanly

Verified against the spec's explicit boundary lists:

- **No new `backend/` tree.** Confirmed — all healthcheck code is in
  `api-server/internal/{api,model,kube}` extending Phase 03 structure.
- **No new CRDs.** Confirmed — read-only access to existing Project/UnitSet/
  Unit CRDs; no CRD creation or patching.
- **No persistent report store.** Confirmed — in-memory `map` keyed by
  `namespace/name`; restart clears it; `HEALTHCHECK_REPORT_NOT_FOUND` is the
  documented contract. `data-architecture.md` updated to reflect this sealed
  decision.
- **No business data mutation.** Confirmed — all DDL targets only
  `upm_healthcheck` database; truncate only on `local_events`; no other
  database touched.
- **Validation-object drift detection.** Confirmed — `SHOW CREATE TABLE`
  checks both `local_events` (must contain `ReplicatedMergeTree` +
  `shard_key UInt64`) and `dist_events` (must contain `Distributed` +
  `upm_cluster`). Mismatch → FAIL with definition evidence, no silent
  overwrite.
- **No gRPC.** Confirmed — zero gRPC imports; all ClickHouse interaction
  through `service-ctl.sh login --query`.
- **Metrics endpoint is WARN severity.** Confirmed — code uses
  `HealthSeverityWarning`; failure status is `HealthStatusWarn`. Spec §6
  explicitly labels this as Phase 04 warning.
- **DDL idempotency.** `CREATE IF NOT EXISTS` + `TRUNCATE` before insert
  handled correctly. No `DROP` statements.

## 7. Findings — Non-blocking

None of these block Phase 05. Tracked for forward visibility.

### F1. `internal/clickhouse` is dead code in Phase 04 (informational)

`api-server/internal/clickhouse/client.go` (52 lines, HTTP query client) was
introduced in Phase 03 as scaffolding for Phase 04 healthcheck SQL — but
Phase 04's implementation uses pod exec + `service-ctl.sh login` instead,
which handles ClickHouse authentication transparently without needing the
admin password in process memory. The HTTP client package is compiled but
never imported.

This was Phase 03 finding F1; the prediction was that Phase 04 would wire it
in. Instead, Phase 04 made a different (better) decision: exec through the
in-pod helper script that already knows the credentials. The HTTP client
should either be removed or explicitly reserved for Phase 05 (Prometheus
query summary).

**Suggested follow-up:** Either delete `internal/clickhouse/` entirely (cleanest
— 52 lines, no callers, compiles but unused) or add a package comment stating
it is reserved for Phase 05 metrics query. Removing it is preferred; it can
always be recovered from git history.

### F2. No unit tests for the healthcheck orchestration in `kube/store.go` (medium)

The healthcheck implementation (`RunHealthcheck` and its 14 `check*` methods,
~300 lines) has zero unit tests. The `fakeStore` in `api/server_test.go`
returns a synthetic report, so the handler/cache layer is tested, but every
`check*` method in `kube/store.go` calls real Kubernetes API clients
(`s.dynamic.Resource(...).Get`, `s.core.CoreV1().Pods(...).List`,
`s.execPod`, etc.) and has no test coverage.

Current coverage comes entirely from:

- Runtime validation (real cluster, real checks).
- Model-level tests (aggregation, secret scanning).
- Handler-level tests (cache, 404, error propagation).

This is acceptable for a hackathon demo where the runtime gate is the primary
quality signal, but the following are untestable without a live cluster:

- PVC count mismatch detection.
- Endpoint readiness parsing.
- Keeper role parsing from `mntr` output.
- `system.clusters` TSV parsing and topology comparison.
- Replica health row parsing and unhealthy detection.
- Drift detection in `SHOW CREATE TABLE` output.

**Suggested follow-up:** Extract the parsing functions (`parseTSV`,
`parseKeeperRole`, `hasColumnDefinition`, `podIsReady`) and the `addFailure`
helper into pure functions testable without any Kubernetes client. Then add
table-driven tests for each parser with real ClickHouse output fixtures.
The `check*` methods themselves can be tested with a mock dynamic client
(similar to `fakeStore` pattern) in a future cleanup pass.

### F3. Secret leak scan covers only three literal patterns (low)

`addSecretLeakageCheck` (`store.go:779-801`) scans the marshaled report for
three lowercase tokens: `clickhouse_admin_password`, `aes_secret_key`,
`secretkeyref`. This catches field-name leaks but would not catch:

- A ClickHouse error string containing the admin password verbatim in a
  `stderr` field (e.g., if ClickHouse echoes credentials in an error message).
- Base64-encoded secrets in evidence fields.
- The admin Secret **name** (e.g., `clickhouse-phase03-secret`) appearing in
  evidence, which is arguably safe but still identifies the credential target.

The first case (password in error output) is the highest-value fix. The
`execPod` function returns `stderr` and `error` strings that are placed into
evidence, and some `check*` methods include `stderr`/`error` in evidence maps.

**Suggested follow-up:** In Phase 05, extend the forbidden-pattern list to
include the admin Secret's data keys (the ones that actually carry password
values), and consider scanning all string values in the evidence map
recursively rather than scanning only the JSON payload. This is not blocking
because: (a) `service-ctl.sh login` uses `--query` which does not echo
credentials, and (b) the runtime script independently greps the saved JSON.

### F4. `checkWriteReadProbe` reference to `cluster.Summary` field (informational)

In `store.go:369-372`, `RunHealthcheck` destructures the cluster summary's
`Topology` field but does not check `cluster.Status` before proceeding.
`GetCluster` returns the summary directly from the UnitSet label set without
validating that the cluster is currently `Running`. A healthcheck on a
provisioning or degraded cluster could produce misleading results (e.g.,
`server_unitset_ready` may pass the UnitSet count but pods are still starting).

In practice, the runtime validation script gates on `kubectl wait` for both
UnitSets to be ready before calling the healthcheck, so this is not a
real issue in the current acceptance flow.

**Suggested follow-up:** Add a `ClusterReady` precondition check at the top of
`RunHealthcheck` that verifies both UnitSets have `readyUnits == specUnits`
before proceeding to individual checks, and return a structured error
(`CLUSTER_NOT_READY`) if the cluster is not in `Running` state. This would
make the healthcheck self-documenting rather than depending on the caller to
pre-check readiness.

## 8. Test Coverage Summary

```text
Package              Tests  Status
internal/api         8      PASS (race clean)
internal/model       6      PASS (race clean)
internal/kube        3      PASS (race clean) — pre-existing Phase 03 tests only
internal/clickhouse  0      no test files (dead code, see F1)
```

Model tests cover: aggregation (PASS+WARN→WARN, WARN+critical FAIL→FAIL),
secret token absence in marshaled JSON, request validation (7 failure
mutations), response model security-field absence.

API tests cover: healthz, validation error, trailing JSON rejection, secret
echo prevention in response and logs, empty list, latest-report cache hit,
latest-report 404 before run.

Kube tests (Phase 03 carry-forward): UnitSet rendering (no password in env),
topology parsing, secret-key validation, cluster-name boundary matching.

## 9. Recommendation — Go / No-Go for Phase 05

**Go.** All 16 acceptance criteria are met with reproducible static evidence
(re-executed in this review: gofmt / vet / test / -race / build / cross-build /
bash -n) and credible runtime evidence (read from a hard-asserting validation
script that independently cross-checks ClickHouse SQL state via kubectl exec).
Scope was held cleanly. The in-memory cache contract is documented in both the
API reference and the data architecture doc. RBAC for pod exec was already
present from Phase 03.

Suggested entry conditions for Phase 05 (none block the start):

1. Remove or repurpose `internal/clickhouse` (F1) — Phase 05 (Prometheus
   query summary) could use it as an HTTP client base, or it should be deleted.
2. Extract and unit-test the healthcheck parsing functions (F2) — these are
   pure functions that don't need Kubernetes and are cheap to cover before
   they accumulate more logic in Phase 05.
3. Extend the secret-leak scan to cover recursive value checking in evidence
   maps (F3) — Phase 05's metrics endpoint will return more external data,
   increasing the surface for accidental credential inclusion.
4. Add a `ClusterReady` precondition to `RunHealthcheck` (F4) to make the
   healthcheck self-documenting about cluster state.

## 10. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-06-06 | Initial review against Phase 04 v0.5; PASS verdict |
