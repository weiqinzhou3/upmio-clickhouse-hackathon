# Phase 04 Review Response and Closeout

- Version: 1.0
- Date: 2026-06-06
- Owner: zqw
- Review artifact: `phase-04-review.md`
- Status: Repairs implemented and runtime validated

## 1. Review Verdict

Claude Code marked Phase 04 as PASS with four non-blocking findings.

Codex accepted the overall PASS direction, but did not immediately close the
phase. A second code/spec comparison found additional gaps that needed repair
before merging Phase 04 into `main`.

## 2. Disposition of Claude Findings

| Finding | Decision | Resolution |
|---|---|---|
| F1: unused `internal/clickhouse` HTTP client | Accepted | Removed the unused package. Phase 05 requires a Prometheus API client, not this ClickHouse HTTP client |
| F2: insufficient kube healthcheck unit tests | Accepted | Added tests for Keeper/TSV parsing, DDL definitions, topology-aware probe SQL, actual Secret-value redaction, and required Service/Endpoint ports |
| F3: secret scan covers only literal field names | Accepted and strengthened | Healthcheck loads the referenced admin/AES Secret values in memory, recursively redacts them from messages/evidence, and verifies the rendered report does not contain the actual or base64-encoded values |
| F4: return `CLUSTER_NOT_READY` before healthcheck | Rejected | A healthcheck must diagnose provisioning/degraded clusters and return individual failed checks; rejecting before checks would reduce diagnostic value |

## 3. Additional Repairs Found During Closeout

| Gap | Risk | Resolution |
|---|---|---|
| Drift validation happened after truncate/insert | A drifted API-owned table could be modified before the healthcheck reported failure | Validate existing definitions before table creation or writes, validate again after create, then run the probe |
| Replica row assertion hardcoded `local_rows != 4` | Probe was only correct for the current 2-shard lab topology | Generate probe row count from topology and validate shard coverage, replica count, non-empty shards, and equal row counts within each shard |
| Service check only validated ready addresses | Required TCP/HTTP/interserver/metrics ports could be missing while the check passed | Validate required ClickHouse Service and Endpoint port names/numbers |
| Report omitted `cluster` and `durationMs` from the Phase spec model | API output differed from the confirmed minimum report | Added `cluster` while retaining `name`, and calculate `durationMs` during finalization |
| Concurrent same-cluster probes could truncate each other's data | Parallel POST requests could produce false failures or inconsistent evidence | Serialize same-cluster healthchecks in `upm-api-server` |
| Runtime script hardcoded 2 shards, 2 replicas, and 8 rows | Runtime acceptance did not prove dynamic topology behavior | Read expected shards, replicas, and inserted rows from the API report |

## 4. Closeout Decision

Phase 04 may close only after:

1. Local Go, Markdown, secret-scan, and Kubernetes manifest gates pass.
2. Updated `upm-api-server` is deployed in Kubernetes.
3. `clickhouse/phase-04/scripts/validate-healthcheck-runtime.sh` passes against
   the real NodePort API and ClickHouse database.
4. Final runtime reports are saved without Secret material.
5. The closeout commit is pushed and the `phase-04` branch is merged into
   `main`.

## 5. Final Evidence

- Go format, unit tests, race tests, vet, and build: PASS.
- Markdown lint and shell syntax: PASS.
- Gitleaks workspace scan: PASS.
- Kubernetes server-side manifest validation: PASS.
- Same-cluster concurrent healthchecks: both PASS.
- Pre-write drift protection: PASS; marker row remained unchanged.
- Final real NodePort runtime validation:

```text
PASS phase04_healthcheck_runtime_validation
```

Final report:

```text
status=PASS passed=14 warnings=0 failed=0
```
