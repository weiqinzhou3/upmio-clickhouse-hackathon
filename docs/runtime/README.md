# Runtime Validation Summary

## Purpose

Summarize UPMIO ClickHouse runtime validation evidence for the future ClickHouse Master Spec.

## Commands executed

Evidence was gathered with:

```bash
kubectl get nodes -o wide
kubectl get sc
helm list -A
kubectl get crd
kubectl api-resources
kubectl get unitsets,units,pods,pvc,svc,endpoints,podmonitor,grpccall -n upm-clickhouse-runtime -o wide
kubectl exec ... clickhouse-client
kubectl exec ... Keeper ruok/mntr
kubectl logs ...
```

## Runtime evidence

| Area | Status | Evidence File | Blocker | Spec Impact |
|---|---|---|---|---|
| UPMIO operators | PASS | [01-upmio-operator-installation.md](01-upmio-operator-installation.md) | image pull unreliable, solved by local import | Spec must include operator/package/image prerequisites |
| UnitSet behavior | PASS | [02-unitset-runtime-behavior.md](02-unitset-runtime-behavior.md) | namespace Project prerequisite | Manager should read UnitSet and Unit status |
| Keeper UnitSet | PASS | [03-clickhouse-keeper-unitset-validation.md](03-clickhouse-keeper-unitset-validation.md) | needs runtime Secret/env/log mount | Keeper can be MVP Day1 dependency |
| ClickHouse UnitSet | PARTIAL | [04-clickhouse-server-unitset-validation.md](04-clickhouse-server-unitset-validation.md) | package config required runtime patch; no distributed_ddl | MVP possible, but template fixes needed |
| Monitoring | PARTIAL | [05-clickhouse-monitoring-runtime-validation.md](05-clickhouse-monitoring-runtime-validation.md) | PodMonitor works, metrics endpoint not listening | Metrics config must be added before dashboards/alerts |
| GrpcCall | FAIL | [06-clickhouse-grpccall-validation.md](06-clickhouse-grpccall-validation.md) | installed operator rejects `type: clickhouse` | Exclude Day2 GrpcCall operations unless image fixed |
| Topology decision | PASS | [07-clickhouse-topology-decision.md](07-clickhouse-topology-decision.md) | 2x2 not represented by current package | MVP should be 1 shard x 2/3 replicas |

## Key findings

UPMIO can run ClickHouse Keeper and ClickHouse Server through `UnitSet`, but the public package and image combination is not fully turnkey.

Runtime passed:

- UPMIO operators installed and running.
- `UnitSet` creates `Unit`, Pod, PVC, Services, ConfigMaps, and PodMonitor when CRDs exist.
- 3-node Keeper ensemble formed leader/follower state.
- 2-replica ClickHouse Server cluster started after runtime ConfigMap patches.
- `ReplicatedMergeTree` replicated data through Keeper.

Runtime failed or partial:

- ClickHouse package template requires fixes for default user auth and `max_concurrent_queries`.
- `ON CLUSTER` DDL is unavailable because `distributed_ddl` is absent.
- PodMonitor is created, but ClickHouse metrics endpoint is not enabled.
- `GrpcCall` for ClickHouse fails with `unsupported unit type "clickhouse"` in installed `unit-operator:v1.1.0`.

## Open questions

- Which operator image tag includes ClickHouse `GrpcCall` support?
- Should the hackathon patch package templates or let the future manager overlay runtime config?
- Is 1 shard x 2 replicas acceptable for MVP, or must the first demo show 2x2?
- Should Prometheus/Grafana be external dependencies or product-managed components?
- What object storage should be used for backup validation?

## Conclusion

Evidence is sufficient to start writing the Master Spec with explicit caveats. Evidence is not sufficient to start coding the ClickHouse manager as if UPMIO ClickHouse support were turnkey.

## Impact on future ClickHouse Spec

The first-stage Spec should define:

- MVP topology: 3 Keeper + 1 shard x 2 replicas.
- UPMIO prerequisites: operators, package charts, Project object, image availability.
- package-template gaps: auth, invalid setting, metrics config, distributed DDL.
- Day2 gap: ClickHouse `GrpcCall` unsupported by installed operator image.
- monitoring split: UPMIO creates PodMonitor, external stack and ClickHouse metrics config still required.
