# UnitSet Runtime Behavior

## Purpose

Validate what `UnitSet` creates at runtime and identify which status and labels a future ClickHouse manager should read.

## Commands executed

```bash
kubectl get unitsets,units,pods,pvc,svc,endpoints -n upm-clickhouse-runtime -o wide
kubectl get unitset clickhouse-runtime -n upm-clickhouse-runtime -o yaml
kubectl get units -n upm-clickhouse-runtime -o yaml
kubectl get unit clickhouse-runtime-0 -n upm-clickhouse-runtime -o jsonpath='{.metadata.labels}{.metadata.ownerReferences}{.status}'
kubectl get pod clickhouse-runtime-0 -n upm-clickhouse-runtime -o jsonpath='{.metadata.labels}{.metadata.ownerReferences}{.spec.containers[*].image}'
kubectl get pvc clickhouse-runtime-0-data -n upm-clickhouse-runtime -o jsonpath='{.metadata.labels}{.metadata.ownerReferences}'
kubectl get events -n upm-clickhouse-runtime --sort-by=.lastTimestamp
```

Runtime object used:

```text
docs/runtime/manifests/clickhouse-keeper-unitset-runtime.yaml
docs/runtime/manifests/clickhouse-unitset-runtime.yaml
```

## Runtime evidence

`UnitSet` objects:

```text
unitset/clickhouse-runtime          TYPE clickhouse          VERSION 26.3.9.8 EXPECTED 2 CURRENT 2 READY 2
unitset/clickhouse-runtime-keeper   TYPE clickhouse-keeper   VERSION 26.3.9.8 EXPECTED 3 CURRENT 3 READY 3
```

`Unit` objects:

```text
unit/clickhouse-runtime-0          PHASE Ready PROCESS STATE starting NODE upm-k8s-worker01 NODE READY True
unit/clickhouse-runtime-1          PHASE Ready PROCESS STATE starting NODE upm-k8s-worker02 NODE READY True
unit/clickhouse-runtime-keeper-0   PHASE Ready PROCESS STATE starting NODE upm-k8s-worker03 NODE READY True
```

Pods:

```text
pod/clickhouse-runtime-0          2/2 Running
pod/clickhouse-runtime-1          2/2 Running
pod/clickhouse-runtime-keeper-0   2/2 Running
pod/clickhouse-runtime-keeper-1   2/2 Running
pod/clickhouse-runtime-keeper-2   2/2 Running
```

PVCs:

```text
clickhouse-runtime-0-data          Bound 20Gi local-path
clickhouse-runtime-1-data          Bound 20Gi local-path
clickhouse-runtime-keeper-0-data   Bound 10Gi local-path
clickhouse-runtime-keeper-1-data   Bound 10Gi local-path
clickhouse-runtime-keeper-2-data   Bound 10Gi local-path
```

Services:

```text
clickhouse-runtime-0-svc                 ClusterIP 9000,8123,9009,9363 selector unit-operator/unit.name=clickhouse-runtime-0
clickhouse-runtime-1-svc                 ClusterIP 9000,8123,9009,9363 selector unit-operator/unit.name=clickhouse-runtime-1
clickhouse-runtime-headless-svc          Headless  9000,8123,9009,9363 selector unit-operator/unitset.name=clickhouse-runtime
clickhouse-runtime-keeper-0-svc          ClusterIP 9181,9234
clickhouse-runtime-keeper-headless-svc   Headless  9181,9234
```

Labels binding UnitSet, Unit, Pod, Service convention:

```json
{
  "unit-operator/unit.name": "clickhouse-runtime-0",
  "unit-operator/unit.sn": "0",
  "unit-operator/unitset.name": "clickhouse-runtime",
  "unit-operator/unitset.units.count": "2",
  "upm.api/service-group.name": "clickhouse-runtime",
  "upm.api/service-group.type": "clickhouse-sg",
  "upm.api/service.type": "clickhouse"
}
```

Owner references:

```text
Unit owner: kind=UnitSet name=clickhouse-runtime
Pod owner:  kind=Unit    name=clickhouse-runtime-0
PVC owner:  none observed
PodMonitor owner: kind=UnitSet name=clickhouse-runtime
```

`Unit.status` evidence:

```json
{
  "configSyncStatus": {"status": "True"},
  "hostIP": "192.168.35.202",
  "nodeName": "upm-k8s-worker01",
  "nodeReady": "True",
  "persistentVolumeClaim": [{"name":"clickhouse-runtime-0-data","phase":"Bound","capacity":{"storage":"20Gi"}}],
  "phase": "Ready",
  "podIPs": [{"ip":"192.168.237.146"}],
  "processState": "starting",
  "task": ""
}
```

Source evidence:

```text
unit-operator/api/v1alpha2/unitset_types.go:79-97 defines storage, emptyDir, extraVolume, certificateProfile, podMonitor.
unit-operator/pkg/controller/unitset/unitset_podmonitor.go:42-65 creates PodMonitor owned by UnitSet.
```

## Key findings

`UnitSet` creates or drives:

- `Unit` resources.
- per-unit Pods through `Unit`.
- PVCs from `spec.storage`.
- per-unit ClusterIP Services and headless UnitSet Service.
- ConfigMaps for config templates and per-unit config values.
- PodMonitor when `spec.podMonitor.enable=true` and PodMonitor CRD exists.

Secrets are not automatically created for application credentials. Runtime validation required a runtime Secret mounted into ClickHouse/agent containers.

Lifecycle observed:

```text
Project -> Namespace RBAC/secret prerequisites
UnitSet -> UnitSet config template/value ConfigMaps
UnitSet -> Units
Unit -> PVC/Pod
UnitSet -> per-unit services/headless service
UnitSet -> PodMonitor, only when Prometheus Operator CRD exists
```

## Open questions

- Should the manager treat `Unit.status.processState=starting` as usable if Pod readiness and SQL health are good?
- Should PVC owner references be added by UPMIO or should cleanup remain explicit?
- Should the future manager directly display `UnitSet.status.readyUnits/currentUnits/expectedUnits` and `Unit.status.configSyncStatus`?

## Conclusion

Status: PASS.

`UnitSet` runtime behavior is sufficient for a manager to create and observe ClickHouse and Keeper instances. The manager should not rely on StatefulSet semantics; it should read `UnitSet`, `Unit`, Pod, PVC, Service, and PodMonitor objects.

## Impact on future ClickHouse Spec

The first-stage Spec should model ClickHouse runtime as UPMIO `UnitSet` plus derived `Unit` resources. Required manager read paths:

- `UnitSet.status.expectedUnits/currentUnits/readyUnits`.
- `Unit.status.phase`, `nodeName`, `nodeReady`, `podIPs`, `persistentVolumeClaim`, `configSyncStatus`.
- Pod container readiness for `clickhouse` and `unit-agent`.
- Service and endpoint records for client access.
