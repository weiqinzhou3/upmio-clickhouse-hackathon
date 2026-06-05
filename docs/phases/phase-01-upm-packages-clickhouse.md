# Phase 01: upm-packages/clickhouse Foundation Adaptation

- Version: 0.4
- Date: 2026-05-27
- Status: Confirmed
- Priority: P0
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - ../evidence-summary.md
  - ../design/upm-packages-clickhouse-design.md
  - ../design/resource-model.md
  - ../design/monitoring-design.md
  - ../design/data-architecture.md
  - ../runtime/03-clickhouse-keeper-unitset-validation.md
  - ../runtime/04-clickhouse-server-unitset-validation.md
  - ../runtime/05-clickhouse-monitoring-runtime-validation.md

## 1. Purpose

Make the ClickHouse and ClickHouse Keeper packages repeatably deployable through UPMIO without manual ConfigMap patching.

This phase produces the first executable baseline for later phases:

- 3 ClickHouse Keeper Units.
- 1 shard x 2 replicas ClickHouse Server.
- Secret-based password initialization.
- Native ClickHouse Prometheus endpoint.
- UPMIO `UnitSet`-based lifecycle.

## 2. Scope

In scope:

1. Fix `upm-packages/clickhouse` package templates and values.
2. Fix `upm-packages/clickhouse-keeper` package templates and values where needed.
3. Add safe password-based initialization for default/admin users.
4. Add required Secret/env/log mount behavior validated by runtime tests.
5. Enable native ClickHouse Prometheus endpoint.
6. Provide 1 shard x 2 replicas + 3 Keeper example values/manifests.
7. Validate the rendered configuration and runtime behavior.

## 3. Non-Goals

Out of scope:

- `upm-api-server` implementation.
- Multi-shard topology support beyond documenting future parameters.
- New compose-operator CRD.
- ClickHouse `GrpcCall` backup/restore/set-variable.
- Full Grafana dashboard or PrometheusRule implementation.
- Host-level ClickHouse installation.
- Use of `<no_password/>` under any condition.

## 4. Required Technical Changes

### 4.1 Password model

The package must not render `<no_password/>`.

Required behavior:

- ClickHouse default/admin password comes from Kubernetes Secret or approved UPMIO Secret mechanism.
- Rendered `users.xml` or equivalent config must contain password-based authentication.
- No plaintext password may appear in ConfigMap, logs, rendered examples, or Git.
- Example manifests must use placeholders only.

### 4.2 Config compatibility

The package must remove or correct settings that failed runtime validation.

Known runtime issue:

- `max_concurrent_queries` was rejected by ClickHouse 26.3.9.8 during access-control profile parsing.

Required behavior:

- Rendered config must be accepted by target ClickHouse version.
- `clickhouse-server --config-file ...` validation should be used when feasible.
- Startup must not require manual ConfigMap patching.

### 4.3 Runtime mounts and environment

Runtime validation proved these are required:

- `Project` object for namespace service account/RBAC/secret prerequisites.
- Secret mount for `AES_SECRET_KEY` and application credentials.
- Explicit `SECRET_MOUNT` and `AES_SECRET_KEY` environment variables for unit-agent.
- Log writable mount for ClickHouse and Keeper logs.

Required behavior:

- Package values expose these settings in a controlled way.
- Runtime manifests include safe defaults or clear required fields.
- Missing Secret or mount produces a clear failure mode documented in README.

### 4.4 Metrics endpoint

MVP uses native ClickHouse Prometheus endpoint, not exporter sidecar.

Required behavior:

- Render ClickHouse `<prometheus>` configuration.
- Expose metrics port consistently, default `9363` unless changed by design.
- `UnitSet.spec.podMonitor.enable=true` can discover the port named `metrics`.
- `curl http://127.0.0.1:9363/metrics` from inside the pod returns metrics.

### 4.5 Keeper package

Required behavior:

- Keeper `UnitSet` can create 3 Ready Keeper Pods.
- Keeper config builds raft configuration from `UNIT_COUNT` and headless service names.
- `ruok` returns `imok` for each Keeper.
- `mntr` shows exactly one leader and two followers in the 3-node baseline.

## 5. Files Likely Changed

Expected files, adjusted to actual repository layout:

```text
upm-packages/clickhouse/<version>/charts/values.yaml
upm-packages/clickhouse/<version>/charts/files/clickhouseTemplate.tpl
upm-packages/clickhouse/<version>/charts/files/users.xml.tpl
upm-packages/clickhouse/<version>/charts/templates/*
upm-packages/clickhouse/<version>/README.md
upm-packages/clickhouse-keeper/<version>/charts/values.yaml
upm-packages/clickhouse-keeper/<version>/charts/files/clickhouseKeeperTemplate.tpl
unit-operator/examples/unitsets/clickhouse-unitset.yaml
unit-operator/examples/unitsets/clickhouse-keeper-unitset.yaml
examples/clickhouse/1s2r-values.yaml
examples/clickhouse/keeper-3-values.yaml
```

If the actual repository uses different file names, keep the same responsibility split.

## 6. Acceptance Criteria

This phase is complete only when all objective criteria below are satisfied or explicitly recorded as skipped with reason.

1. Rendered ClickHouse config contains no `<no_password/>`.
2. Rendered ClickHouse config does not include the known invalid `max_concurrent_queries` profile setting.
3. Rendered ClickHouse config includes password-based authentication sourced from Secret placeholders.
4. Rendered ClickHouse config includes native Prometheus endpoint configuration.
5. Keeper `UnitSet` reaches expected/current/ready = 3/3/3.
6. ClickHouse Server `UnitSet` reaches expected/current/ready = 2/2/2.
7. `SELECT 1` succeeds from at least one ClickHouse pod.
8. `system.clusters` shows `upm_cluster` with 1 shard and 2 replicas.
9. `system.replicas` for the runtime validation table shows:
   - `is_readonly = 0`
   - `is_session_expired = 0`
   - `absolute_delay = 0` or acceptable near-zero value
   - `queue_size = 0` after convergence
10. Keeper health checks pass:
    - `ruok -> imok` on all Keeper pods
    - `mntr` shows one leader and two followers
11. `curl http://127.0.0.1:9363/metrics` returns Prometheus metrics from a ClickHouse pod.
12. No real secrets appear in generated files, logs, reports, or commits.
13. Runtime no longer requires manual ConfigMap patching.

## 7. Verification Commands

Run commands adapted to the actual namespace/release names.

```bash
# Render packages
helm lint upm-packages/clickhouse/<version>/charts
helm template clickhouse-runtime upm-packages/clickhouse/<version>/charts \
  --values examples/clickhouse/1s2r-values.yaml >/tmp/clickhouse-rendered.yaml

helm lint upm-packages/clickhouse-keeper/<version>/charts
helm template clickhouse-keeper-runtime upm-packages/clickhouse-keeper/<version>/charts \
  --values examples/clickhouse/keeper-3-values.yaml >/tmp/clickhouse-keeper-rendered.yaml

# Static checks
grep -R "<no_password" /tmp/clickhouse-rendered.yaml && exit 1 || true
grep -R "max_concurrent_queries" /tmp/clickhouse-rendered.yaml && exit 1 || true
grep -R "<prometheus>" /tmp/clickhouse-rendered.yaml

# Kubernetes validation
kubectl apply --dry-run=server -f /tmp/clickhouse-keeper-rendered.yaml
kubectl apply --dry-run=server -f /tmp/clickhouse-rendered.yaml

# Runtime validation
kubectl get unitsets,units,pods,pvc,svc -n upm-clickhouse-runtime -o wide
kubectl exec -n upm-clickhouse-runtime <keeper-pod> -c clickhouse-keeper -- bash -lc 'echo ruok | nc 127.0.0.1 9181'
kubectl exec -n upm-clickhouse-runtime <keeper-pod> -c clickhouse-keeper -- bash -lc 'echo mntr | nc 127.0.0.1 9181'
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query 'SELECT 1'
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query "SELECT cluster, shard_num, replica_num, host_name, port FROM system.clusters WHERE cluster='upm_cluster' ORDER BY shard_num, replica_num"
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query "SELECT database, table, is_readonly, is_session_expired, absolute_delay, queue_size FROM system.replicas FORMAT Vertical"
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- curl -sS --max-time 5 http://127.0.0.1:9363/metrics | head
```

## 8. Risks and Open Questions

| Risk / Question | Handling |
|---|---|
| Target ClickHouse version changes after package update | Keep version-specific package directory and verify rendered config per version |
| Secret model conflicts with existing UPMIO convention | Follow MySQL package pattern and document final Secret key names |
| Prometheus endpoint exposes insufficient metrics | Keep exporter sidecar as future option, not MVP |
| Local registry/image import blocks runtime validation | Record as environment risk and use private registry or local import |

## 9. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-05-27 | Initial phase draft |
| 0.2 | 2026-05-27 | Added package/runtime validation details |
| 0.3 | 2026-05-27 | Added metadata and red-team fix structure |
| 0.4 | 2026-05-27 | Restored phase-specific technical requirements, file targets, objective acceptance criteria, and ClickHouse runtime verification commands |
