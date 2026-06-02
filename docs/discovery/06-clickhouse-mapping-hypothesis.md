# Phase 06 ClickHouse Mapping Hypothesis

## Purpose

Propose an evidence-based hypothesis for mapping a future ClickHouse HA target onto current public UPMIO objects. This is not a final design and does not implement any product code.

## Commands Executed

```bash
rg -n "clickhouse|clickhouse-keeper|remote_servers|zookeeper|keeper|GrpcCall|logical-backup|restore|set-variable|service-group" unit-operator upm-packages compose-operator -S
find unit-operator/examples upm-packages/clickhouse upm-packages/clickhouse-keeper -type f | sort
kubectl get nodes -o wide
kubectl get sc
```

## Key Findings

Target hypothesis:

```text
ClickHouse HA Cluster
├── 2 shards x 2 replicas = 4 ClickHouse Server instances
└── 3 ClickHouse Keeper instances
```

Evidence supports a first MVP using existing UPMIO package and UnitSet mechanisms:

- ClickHouse Server maps naturally to `UnitSet` with `type: clickhouse`; one `Unit` per server pod.
- ClickHouse Keeper maps naturally to a separate `UnitSet` with `type: clickhouse-keeper`.
- Current public ClickHouse package explicitly supports Keeper as a separate UnitSet in the same service group.
- Current public ClickHouse package renders one shard with multiple replicas from `UNIT_COUNT`.
- Current template hardcodes `<shard>01</shard>`, so 2-shard topology is not yet directly supported by the inspected package template.
- `GrpcCall` already routes ClickHouse `logical-backup`, `restore`, and `set-variable` actions to the ClickHouse unit-agent.
- No public `ServiceGroup` CRD was found; service group is currently a label/package convention in public repos.
- No ClickHouse-specific compose-operator CRD equivalent to `MysqlReplication` was found.

## Evidence

ClickHouse package README evidence from `upm-packages/clickhouse/README.md`:

```text
Supported UPM package version:
- clickhouse/26.3.9.8
ClickHouse Keeper is provisioned as a separate UnitSet in the same service group.
The first supported topology is one shard with two or three ClickHouse replicas.
```

ClickHouse UnitSet evidence from `unit-operator/examples/unitsets/clickhouse-unitset.yaml`:

```yaml
kind: UnitSet
metadata:
  name: clickhouse-sample
  labels:
    upm.api/service-group.name: clickhouse-sample
    upm.api/service-group.type: clickhouse-sg
    upm.api/service.type: clickhouse
spec:
  type: clickhouse
  version: 26.3.9.8
  units: 2
```

Keeper UnitSet evidence from `unit-operator/examples/unitsets/clickhouse-keeper-unitset.yaml`:

```yaml
kind: UnitSet
metadata:
  name: clickhouse-keeper-sample
  labels:
    upm.api/service-group.name: clickhouse-sample
    upm.api/service-group.type: clickhouse-sg
    upm.api/service.type: clickhouse-keeper
spec:
  type: clickhouse-keeper
  version: 26.3.9.8
  units: 3
```

Topology template evidence from `upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl`:

```xml
<remote_servers>
  <upm_cluster>
    <shard>
      <internal_replication>true</internal_replication>
      ...
      <replica>
        <host>{{ $serviceName }}-{{ $i }}.{{ $serviceName }}-headless-svc.{{ $namespace }}.svc.cluster.local</host>
        <port>{{ $tcpPort }}</port>
      </replica>
    </shard>
  </upm_cluster>
</remote_servers>
<zookeeper>
  <node>
    <host>{{ $keeperServiceName }}-{{ $i }}.{{ $keeperServiceName }}-headless-svc.{{ $namespace }}.svc.cluster.local</host>
    <port>{{ $keeperPort }}</port>
  </node>
</zookeeper>
<macros>
  <shard>01</shard>
  <replica>{{ getenv "POD_NAME" }}</replica>
</macros>
```

ClickHouse operational task evidence from `unit-operator/pkg/controller/grpccall/handler.go`:

```go
case upmv1alpha1.ClickHouseType:
    switch instance.Spec.Action {
    case upmv1alpha1.LogicalBackupAction:
        return chc.LogicalBackup(...)
    case upmv1alpha1.RestoreAction:
        return chc.Restore(...)
    case upmv1alpha1.SetVariableAction:
        return chc.SetVariable(...)
    }
```

ClickHouse backup/restore examples:

```yaml
# unit-operator/examples/operations/clickhouse-logical-backup.yaml
kind: GrpcCall
spec:
  targetUnit: clickhouse-sample-0
  type: clickhouse
  action: logical-backup

# unit-operator/examples/operations/clickhouse-restore.yaml
kind: GrpcCall
spec:
  targetUnit: clickhouse-sample-0
  type: clickhouse
  action: restore
```

Prepared cluster evidence:

```text
4 Kubernetes nodes are Ready.
Default StorageClass is local-path.
No UPMIO CRDs are installed yet.
```

## Required Questions

1. ClickHouse Server should map to `UnitSet(type=clickhouse)` and `Unit` for each server instance.
2. ClickHouse Keeper should map to `UnitSet(type=clickhouse-keeper)` and `Unit` for each Keeper instance.
3. Shard/replica topology is currently package-template based. One-shard multi-replica is evidenced; 2-shard mapping is not yet supported by the inspected template without changes.
4. No existing public UPMIO cluster-level object suitable for all ClickHouse cluster semantics was found.
5. There is no public ClickHouse equivalent of `MysqlReplication` in compose-operator.
6. ClickHouse can likely be implemented first as package/chart plus upper-level manager for one-shard/multi-replica without modifying unit-operator and compose-operator.
7. `upm-packages` likely covers image/chart/config template changes, Keeper endpoints, server templates, and metric port exposure.
8. A new upper-level Go API server likely covers workflow orchestration, user-facing APIs, validation, service group conventions, and day-1/day-2 operation coordination.
9. Future compose-operator changes may be needed for declarative multi-shard topology, automatic topology reconciliation, distributed DDL coordination, and status aggregation.
10. MVP should exclude full automatic resharding, distributed table DDL automation, transparent failover semantics, advanced backup scheduling, dashboards/alerts, and arbitrary user lifecycle unless explicitly required.

## Required Output Table

| ClickHouse Concept | Proposed UPMIO Mapping | Confidence | Evidence | Open Question |
|---|---|---|---|---|
| ClickHouse Server | `UnitSet(type=clickhouse)` plus `Unit` | high | `clickhouse-unitset.yaml` | Whether one UnitSet per shard is needed for 2-shard MVP |
| ClickHouse Keeper | `UnitSet(type=clickhouse-keeper)` plus `Unit` | high | `clickhouse-keeper-unitset.yaml` | Whether Keeper lifecycle should be shared across clusters |
| shard | package template macros or future topology object | low | template hardcodes `<shard>01</shard>` | How to represent 2 shards cleanly |
| replica | Unit index inside ClickHouse UnitSet | medium | template loops over `UNIT_COUNT` replicas | Whether replica identity must survive scaling/replacement |
| cluster | labels plus package conventions; maybe future manager object | medium | `upm.api/service-group.*` labels | Is private `ServiceGroup` available? |
| Distributed table | exclude from MVP or manager-owned SQL task | low | no UPMIO CRD evidence | Who owns DDL lifecycle and idempotency? |
| ReplicatedMergeTree | ClickHouse SQL/user workload convention | low | Keeper endpoints/macros exist | Should manager generate database/table DDL? |
| metrics | UnitSet PodMonitor + package metrics endpoint | medium | `PodMonitorInfo`; ClickHouse metrics port references | Which endpoint/exporter is authoritative? |
| backup | `GrpcCall(type=clickhouse, action=logical-backup)` | high | ClickHouse GrpcCall examples and handler | Which storage backend and credential model? |
| user management | package config or future task/API | low | no ClickHouse user CRD found | MVP needs declarative users or manual SQL? |

## Open Questions

- Is one `UnitSet` per shard acceptable, or should a single UnitSet represent all replicas across shards?
- Should the first hackathon demo target current package-supported one shard with two or three replicas instead of 2x2?
- Should a future ClickHouse manager create UPMIO objects only, or also execute SQL/DDL tasks?
- Where should backup credentials live: Kubernetes Secret, UPMIO encrypted secret convention, or external secret manager?

## Conclusion

Current public UPMIO evidence supports ClickHouse one-shard/multi-replica plus Keeper through UnitSets and packages. The requested 2-shard x 2-replica topology is a reasonable future target but is not directly evidenced by the current ClickHouse template.

## Impact on Future ClickHouse Spec

The Master Spec should split MVP from later topology work. MVP should validate package-driven ClickHouse Server + Keeper deployment, service exposure, metrics endpoint, and GrpcCall backup/restore. Multi-shard topology should be a separate design section with explicit decision points for package changes versus a new compose-operator CRD.
