# Phase 04 Runtime Validation

- Date: 2026-06-06
- Namespace: `upm-clickhouse-phase03-runtime`
- Cluster: `clickhouse-phase03`
- API endpoint: `http://192.168.35.201:30083`
- API server image: `localhost/upmio/upm-api-server:phase-04`

## Commands

```bash
go fmt ./...
go test ./...
go test -race ./...
go vet ./...
go build ./...
SSH_PASSWORD=root ./clickhouse/sync-upm-api-server-image-to-nodes.sh
kubectl apply -f clickhouse/phase-03/manifests/upm-api-server.yaml
kubectl rollout restart deploy/upm-api-server -n upm-system
kubectl rollout status deploy/upm-api-server -n upm-system --timeout=180s
kubectl auth can-i create pods --subresource=exec \
  -n upm-clickhouse-phase03-runtime \
  --as=system:serviceaccount:upm-system:upm-api-server \
  --as-group=system:serviceaccounts \
  --as-group=system:serviceaccounts:upm-system \
  --as-group=system:authenticated
clickhouse/phase-04/scripts/validate-healthcheck-runtime.sh
```

## Result

`clickhouse/phase-04/scripts/validate-healthcheck-runtime.sh` completed with:

```text
PASS phase04_healthcheck_runtime_validation
```

Saved reports:

- `clickhouse/phase-04/runtime-healthcheck-report.json`
- `clickhouse/phase-04/runtime-healthcheck-latest.json`

Runtime report summary:

```json
{
  "cluster": "clickhouse-phase03",
  "name": "clickhouse-phase03",
  "durationMs": 6312,
  "status": "PASS",
  "summary": {
    "passed": 14,
    "warnings": 0,
    "failed": 0,
    "skipped": 0
  }
}
```

Service/Endpoint evidence confirmed these required ClickHouse ports:

```text
tcp=9000 http=8123 interserver=9009 metrics=9363
```

Critical checks passed:

- `upmio_project_exists`
- `keeper_unitset_ready`
- `server_unitset_ready`
- `pods_ready`
- `pvc_bound`
- `services_endpoints`
- `keeper_ruok`
- `keeper_leader_follower`
- `clickhouse_select_1`
- `system_clusters_topology`
- `write_read_probe`
- `replica_health`
- `no_secret_leakage`

Warning-level checks passed:

- `metrics_endpoint`

Database cross-check:

```bash
kubectl exec -n upm-clickhouse-phase03-runtime clickhouse-phase03-0 -c clickhouse -- \
  service-ctl.sh login --query 'SELECT count() FROM upm_healthcheck.dist_events FORMAT TSV'
```

Expected and observed output:

```text
8
```

Secret safety check:

```bash
grep -Eino 'CLICKHOUSE_ADMIN_PASSWORD|AES_SECRET_KEY|secretKeyRef' \
  clickhouse/phase-04/runtime-healthcheck-report.json \
  clickhouse/phase-04/runtime-healthcheck-latest.json
```

Expected and observed output: no matches.

## Review Repair Validation

Same-cluster concurrency validation started two NodePort healthchecks
simultaneously. Both completed successfully:

```text
PASS insertedRows=8
PASS insertedRows=8
```

Pre-write drift protection was validated using only the reserved
`upm_healthcheck` database:

1. Replaced the reserved local validation table with an intentionally
   incompatible definition.
2. Inserted one marker row.
3. Called the healthcheck API.
4. Confirmed the report failed at the pre-write drift check.
5. Confirmed the marker row count remained `1`.
6. Removed the reserved test database and reran the final runtime validation.

Observed result:

```text
PASS pre_write_drift_protection marker_rows=1
```

The final runtime validation recreated the correct API-owned validation
objects and returned `PASS phase04_healthcheck_runtime_validation`.

## Notes

The first runtime attempt failed because the ClickHouse Keeper container does
not include `nc`; the healthcheck implementation now uses bash `/dev/tcp` for
Keeper four-letter-command probes. This keeps the check in the existing runtime
container and does not require adding host-level tools or modifying Keeper
images.
