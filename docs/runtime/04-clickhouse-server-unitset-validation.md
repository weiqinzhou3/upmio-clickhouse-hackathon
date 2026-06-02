# ClickHouse Server UnitSet Validation

## Purpose

Validate whether ClickHouse Server can run through UPMIO `UnitSet`, connect to Keeper, and support basic replicated-table SQL.

## Commands executed

```bash
kubectl apply -f docs/runtime/manifests/clickhouse-unitset-runtime.yaml
kubectl get unitsets,units,pods,pvc,svc,endpoints -n upm-clickhouse-runtime -o wide
kubectl logs -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse --tail=120
kubectl logs -n upm-clickhouse-runtime <clickhouse-pod> -c unit-agent --tail=160
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query 'SELECT 1'
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query 'SELECT cluster, shard_num, replica_num, host_name, port FROM system.clusters'
kubectl exec -n upm-clickhouse-runtime <clickhouse-pod> -c clickhouse -- clickhouse-client --query 'SELECT database, table, is_readonly, is_session_expired, absolute_delay, queue_size FROM system.replicas FORMAT Vertical'
```

Runtime patch commands used because the generated package template failed on ClickHouse 26.3.9.8:

```bash
kubectl get cm clickhouse-runtime-config-template -n upm-clickhouse-runtime -o go-template='{{ index .data "clickhouse" }}' > /tmp/clickhouse-runtime-template.patched
perl -0pi -e 's#(<access_management>1</access_management>\n)#$1      <no_password/>\n#' /tmp/clickhouse-runtime-template.patched
perl -0pi -e 's#\n      <max_concurrent_queries>\{\{ getv "/settings/max_concurrent_queries" \}\}</max_concurrent_queries>##' /tmp/clickhouse-runtime-template.patched
jq -n --rawfile c /tmp/clickhouse-runtime-template.patched '{data:{clickhouse:$c}}' > /tmp/clickhouse-runtime-template-patch.json
kubectl patch cm clickhouse-runtime-config-template -n upm-clickhouse-runtime --type=merge --patch-file /tmp/clickhouse-runtime-template-patch.json
kubectl delete pod -n upm-clickhouse-runtime -l unit-operator/unitset.name=clickhouse-runtime
```

Manifest used:

```text
docs/runtime/manifests/clickhouse-unitset-runtime.yaml
```

## Runtime evidence

Final runtime objects:

```text
unitset/clickhouse-runtime TYPE clickhouse VERSION 26.3.9.8 EXPECTED 2 CURRENT 2 READY 2
unit/clickhouse-runtime-0  Ready upm-k8s-worker01
unit/clickhouse-runtime-1  Ready upm-k8s-worker02
pod/clickhouse-runtime-0   2/2 Running
pod/clickhouse-runtime-1   2/2 Running
```

PVCs and services:

```text
clickhouse-runtime-0-data Bound 20Gi local-path
clickhouse-runtime-1-data Bound 20Gi local-path
clickhouse-runtime-0-svc        9000/TCP,8123/TCP,9009/TCP,9363/TCP
clickhouse-runtime-1-svc        9000/TCP,8123/TCP,9009/TCP,9363/TCP
clickhouse-runtime-headless-svc 9000/TCP,8123/TCP,9009/TCP,9363/TCP
```

SQL health:

```text
SELECT 1 -> 1
```

Topology from `system.clusters`:

```text
cluster: upm_cluster shard_num: 1 replica_num: 1 host_name: clickhouse-runtime-0.clickhouse-runtime-headless-svc.upm-clickhouse-runtime.svc.cluster.local port: 9000
cluster: upm_cluster shard_num: 1 replica_num: 2 host_name: clickhouse-runtime-1.clickhouse-runtime-headless-svc.upm-clickhouse-runtime.svc.cluster.local port: 9000
```

`ON CLUSTER` DDL failure:

```text
Code: 139. DB::Exception: There is no DistributedDDL configuration in server config. (NO_ELEMENTS_IN_CONFIG)
```

Replicated table validation was done manually per pod:

```text
query from clickhouse-runtime-0 -> 1 runtime-ok
query from clickhouse-runtime-1 -> 1 runtime-ok
```

`system.replicas`:

```text
database: runtime_validation
table: events
is_readonly: 0
is_session_expired: 0
absolute_delay: 0
queue_size: 0
```

Keeper connection evidence from logs:

```text
ZooKeeperClient: Connected to ZooKeeper at 192.168.237.142:9181 with session_id 1
runtime_validation.events: Became leader
```

Template evidence:

```text
upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl:20-32 defines one <shard> and loops replicas from UNIT_COUNT.
upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl:48-50 hardcodes <shard>01</shard> and replica from POD_NAME.
upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl:59 includes max_concurrent_queries.
upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl:63-70 defines default user without password/no_password.
```

## Key findings

ClickHouse Server can run under `UnitSet` and connect to Keeper after runtime config patching.

Current public package is not cleanly turnkey on ClickHouse 26.3.9.8:

- `default` user lacks `password`, `password_sha256_hex`, or `no_password`.
- `max_concurrent_queries` is rejected as an unknown setting during access-control profile parsing.
- `ON CLUSTER` DDL is unavailable because `distributed_ddl` is not configured.

Client access path:

```text
Native TCP: clickhouse-runtime-0-svc.upm-clickhouse-runtime.svc.cluster.local:9000
HTTP:       clickhouse-runtime-0-svc.upm-clickhouse-runtime.svc.cluster.local:8123
Headless:   clickhouse-runtime-headless-svc for per-replica DNS
```

## Open questions

- Should the package template be fixed before hackathon MVP, or should the manager apply a runtime config overlay?
- Should distributed DDL be added for MVP?
- Should the manager expose one per-unit service or generate a stable read/write service abstraction?

## Conclusion

Status: PARTIAL.

Runtime proves `UnitSet` can run a 1-shard x 2-replica ClickHouse cluster with Keeper-backed `ReplicatedMergeTree`. However, public package template fixes are required for reproducible deployment without manual ConfigMap patching.

## Impact on future ClickHouse Spec

The first-stage Spec should not claim the current package is fully ready. It should include:

- package-template fixes as prerequisites or manager-owned overlays.
- explicit 1-shard x N-replica topology support.
- absence of distributed DDL as an MVP limitation unless fixed.
- service naming and client access conventions.
