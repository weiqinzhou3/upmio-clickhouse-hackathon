# Resource Model

- Version: 0.3
- Date: 2026-05-27
- Status: Sealed
- Owner: zqw
- Related:
  - ../master-spec.md
  - ../architecture/upmio-architecture.md

## 1. Purpose

This document defines how the ClickHouse Manager models clusters using UPMIO and Kubernetes resources.

## 2. MVP Resource Set

| Resource | Type | Provider | Purpose |
|---|---|---|---|
| Project | UPMIO CRD | unit-operator | namespace prerequisites, service account, RBAC, secret baseline |
| Keeper UnitSet | UPMIO CRD | unit-operator | 3-node ClickHouse Keeper ensemble |
| Server UnitSet | UPMIO CRD | unit-operator | ClickHouse Server instances |
| Unit | UPMIO CRD | unit-operator | generated per instance |
| Secret | Kubernetes | Kubernetes | passwords, AES key, runtime credentials |
| ConfigMap | Kubernetes | UPMIO/package | rendered ClickHouse/Keeper configuration |
| Pod | Kubernetes | unit-operator | runtime container group |
| PVC | Kubernetes | unit-operator | persistent data volume |
| Service | Kubernetes | unit-operator | per-unit and headless access |
| PodMonitor | Prometheus Operator CRD | unit-operator | metrics discovery |
| GrpcCall | UPMIO CRD | unit-operator | future Day2 tasks; not MVP dependency |

## 3. Manager Logical Model

The Manager may expose a higher-level logical model such as:

```yaml
cluster:
  name: ch-demo
  namespace: upm-clickhouse
topology:
  shards: 1
  replicasPerShard: 2
keeper:
  replicas: 3
version: 26.3.x
storage:
  dataSize: 100Gi
metrics:
  enabled: true
security:
  adminSecretRef: ch-demo-admin
```

This logical model is not necessarily a new Kubernetes CRD in MVP. It can be API input that renders UPMIO/Kubernetes resources.

## 4. Mapping

| Logical Concept | MVP Mapping | Future Mapping |
|---|---|---|
| Cluster | labels + Manager aggregation | possible ClickHouseCluster CRD |
| Keeper ensemble | `UnitSet(type=clickhouse-keeper)` | same, or topology CRD-owned |
| ClickHouse servers | `UnitSet(type=clickhouse)` | topology-aware UnitSets |
| Replica | Unit index within topology | explicit replica model |
| Shard | package template and future topology params | topology CRD |
| Healthcheck result | Manager generated report | persisted report object/audit store |
| Backup task | Future | GrpcCall or dedicated task model |

## 5. Status Read Path

Manager should read:

- `UnitSet.status` for desired/current/ready units;
- `Unit.status` for phase, node, pod IPs, PVCs, config sync;
- Pod readiness for container-level health;
- PVC phase/capacity;
- Service/Endpoint readiness;
- PodMonitor existence;
- ClickHouse SQL system tables;
- Prometheus targets/metrics.

## 6. Security Rules

- Passwords must be in Secret, not ConfigMap.
- Reports must redact credentials.
- Example YAML must use placeholders.
- Manager must not log Secret values.

## 7. Non-Goals

- MVP does not introduce a new ClickHouse CRD.
- MVP does not reimplement UPMIO reconciliation.
- MVP does not persist a full operation history database.
