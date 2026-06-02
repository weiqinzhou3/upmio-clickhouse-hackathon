# Phase 03 UPMIO CRD Model Analysis

## Purpose

Analyze UPMIO CRD/API models and determine how the current public model may map to ClickHouse server, Keeper, topology, service endpoints, and operational tasks.

## Commands Executed

```bash
find unit-operator -type f | grep -E "api|crd|types|yaml|yml" | sort
find compose-operator -type f | grep -E "api|crd|types|yaml|yml" | sort
grep -R "kind: CustomResourceDefinition" -n unit-operator compose-operator upm-packages || true
grep -R "type .*Spec struct" -n unit-operator/api compose-operator/api || true
grep -R "type .*Status struct" -n unit-operator/api compose-operator/api || true
kubectl get crd | grep -Ei "upm|unit|compose|mysql|redis|postgres|proxy|servicegroup" || true
kubectl api-resources | grep -Ei "upm|unit|compose|mysql|redis|postgres|proxy|servicegroup" || true
kubectl get pods -A | grep -Ei "upm|operator|unit|compose" || true
```

## Key Findings

- `Unit` is the single-instance object. It contains pod template, PVC templates, config template/value references, startup flag, and per-instance status.
- `UnitSet` is the instance-group object. It defines type, edition, version, unit count, resources, services, storage, update strategy, anti-affinity, certificates, and optional PodMonitor.
- `GrpcCall` is the operational task object. It routes typed actions to a unit-agent and records result/message/timestamps in status.
- `compose-operator` provides topology CRDs for MySQL, MySQL Group Replication, ProxySQL sync, Redis, PostgreSQL, MongoDB, and others.
- No `ServiceGroup` CRD or `ServiceGroupSpec` was found in public repos or the prepared cluster. The concept appears as labels and package metadata rather than a first-class public CRD.
- The prepared cluster has no UPMIO CRDs because UPMIO operators were not installed in this phase.

## Evidence

`Unit` spec/status evidence from `unit-operator/api/v1alpha2/unit_types.go`:

```go
type UnitSpec struct {
    Startup bool `json:"startup,omitempty"`
    ConfigTemplateName string `json:"configTemplateName,omitempty"`
    ConfigValueName string `json:"configValueName,omitempty"`
    VolumeClaimTemplates []UnitVolumeClaimTemplate `json:"volumeClaimTemplates,omitempty"`
    Template UnitPodTemplateSpec `json:"template,omitempty"`
    FailedPodRecoveryPolicy *FailedPodRecoveryPolicy `json:"failedPodRecoveryPolicy,omitempty"`
}

type UnitStatus struct {
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`
    Phase UnitPhase `json:"phase"`
    NodeReady string `json:"nodeReady"`
    NodeName string `json:"nodeName"`
    Task string `json:"task"`
    ProcessState string `json:"processState"`
}
```

`UnitSet` spec/status evidence from `unit-operator/api/v1alpha2/unitset_types.go`:

```go
type UnitSetSpec struct {
    Type string `json:"type,omitempty"`
    Edition string `json:"edition,omitempty"`
    Version string `json:"version,omitempty"`
    Units int `json:"units,omitempty"`
    Resources corev1.ResourceRequirements `json:"resources,omitempty"`
    ExternalService ExternalServiceSpec `json:"externalService,omitempty"`
    UnitService UnitServiceSpec `json:"unitService,omitempty"`
    UpdateStrategy UpdateStrategySpec `json:"updateStrategy,omitempty"`
    PodAntiAffinityPreset string `json:"podAntiAffinityPreset,omitempty"`
    Storage []StorageSpec `json:"storage,omitempty"`
    PodMonitor PodMonitorInfo `json:"podMonitor,omitempty"`
}

type UnitSetStatus struct {
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`
    Units int `json:"units,omitempty"`
    ReadyUnits int `json:"readyUnits"`
}
```

`GrpcCall` evidence from `unit-operator/api/v1alpha1/grpccall_types.go`:

```go
ClickHouseType UnitType = "clickhouse"

type Action string
// Enum includes logical-backup, physical-backup, restore, gtid-purge, set-variable, clone, backup

type GrpcCallSpec struct {
    TargetUnit string `json:"targetUnit"`
    Type UnitType `json:"type"`
    Action Action `json:"action"`
    TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished"`
    Parameters map[string]apiextensionsv1.JSON `json:"parameters"`
}

type GrpcCallStatus struct {
    Result Result `json:"result"`
    Message string `json:"message"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`
    StartTime *metav1.Time `json:"startTime,omitempty"`
}
```

`compose-operator` MySQL topology evidence:

```go
// compose-operator/api/v1alpha1/mysqlreplication_types.go
type MysqlReplicationSpec struct {
    Mode MysqlReplicationMode `json:"mode"`
    Secret MysqlReplicationSecret `json:"secret"`
    Source *CommonNode `json:"source"`
    Service *Service `json:"service"`
    Replica ReplicaNodes `json:"replica,omitempty"`
}

type MysqlReplicationStatus struct {
    Topology MysqlReplicationTopology `json:"topology"`
    ReadWriteService string `json:"readwriteService"`
    ReadOnlyService string `json:"readonlyService,omitempty"`
    Ready bool `json:"ready"`
}
```

`ServiceGroup` evidence:

```text
grep -R "type .*ServiceGroup.*struct" unit-operator compose-operator upm-packages
# no result
grep -R "kind: ServiceGroup" unit-operator compose-operator upm-packages
# no result
unit-operator/examples/unitsets/clickhouse-unitset.yaml: labels include upm.api/service-group.name and upm.api/service-group.type
upm-packages/clickhouse/README.md: Keeper is provisioned as a separate UnitSet in the same service group
```

Runtime cluster evidence:

```text
kubectl get crd | grep -Ei "upm|unit|compose|mysql|redis|postgres|proxy|servicegroup" || true
# no UPMIO CRDs installed
kubectl api-resources | grep -Ei "upm|unit|compose|mysql|redis|postgres|proxy|servicegroup" || true
# no UPMIO API resources installed
```

## Output Table

| Concept | Current UPMIO Object | Evidence | Can Map to ClickHouse? | Notes |
|---|---|---|---|---|
| single instance | `Unit` | `unit-operator/api/v1alpha2/unit_types.go` | yes | Maps to one ClickHouse Server pod or one Keeper pod |
| instance group | `UnitSet` | `unit-operator/api/v1alpha2/unitset_types.go` | yes | Maps to a homogeneous ClickHouse Server group or Keeper ensemble |
| database cluster | label-based service group plus package conventions | `upm.api/service-group.name`, `upm-packages/clickhouse/README.md` | partial | No public first-class `ServiceGroup` CRD found |
| replication topology | `MysqlReplication`, `MysqlGroupReplication` | `compose-operator/api/v1alpha1/*mysql*types.go` | maybe | MySQL-specific; useful pattern but not reusable directly |
| operational task | `GrpcCall` | `unit-operator/api/v1alpha1/grpccall_types.go` | yes | Already includes `clickhouse` type and actions |

## Open Questions

- Is a private or enterprise `ServiceGroup` CRD used by UPMIO outside these public repositories?
- Should ClickHouse shard/replica topology become a new compose-operator CRD, or remain package-driven in MVP?
- Should Keeper lifecycle be managed as a separate UnitSet permanently?

## Conclusion

The public UPMIO CRD model has clear lower-level lifecycle primitives (`Unit`, `UnitSet`) and task execution (`GrpcCall`). It does not expose a public generic cluster CRD. MySQL topology is modeled through database-specific compose CRDs.

## Impact on Future ClickHouse Spec

ClickHouse MVP should map server and Keeper processes to UnitSets, use labels for grouping, and use GrpcCall for day-2 tasks. A dedicated ClickHouse topology CRD should be treated as a later extension unless MVP requirements require multi-shard reconciliation beyond package templating.
