# Phase 06 Runtime Evidence

This directory contains Phase 06 Day2 read-only diagnostics validation assets.

## Runtime Target

```text
API Server: http://192.168.35.201:30083
Namespace:  upm-clickhouse-phase03-runtime
Cluster:    clickhouse-phase03
```

## Validation

```bash
clickhouse/phase-06/scripts/validate-diagnostics-runtime.sh
```

Expected final line:

```text
PASS phase06_diagnostics_runtime_validation
```

The script validates the real runtime chain:

1. `upm-api-server` is rolled out in Kubernetes.
2. `GET /api/v1/clusters/{namespace}/{name}/diagnostics` returns `200`.
3. Required findings exist:
   - `replica`
   - `replication_queue`
   - `parts`
   - `merges`
   - `mutations`
   - `keeper`
   - `storage`
   - `write_client_stats`
   - `write_quality`
4. The healthy lab cluster has no `CRITICAL` diagnostics findings.
5. If `system.query_log` is absent, `write_client_stats` returns `UNKNOWN`
   instead of failing the API request.
6. Read-only row-count validation works when `database`, `table`, and
   `expectedRows` are supplied.
7. Diagnostics output does not contain secret-like tokens.

## Saved Evidence

The validation script saves:

```text
clickhouse/phase-06/diagnostics.json
clickhouse/phase-06/diagnostics-rowcount.json
clickhouse/phase-06/system-replicas.tsv
clickhouse/phase-06/system-parts.tsv
clickhouse/phase-06/system-mutations.tsv
clickhouse/phase-06/system-merges.tsv
```
