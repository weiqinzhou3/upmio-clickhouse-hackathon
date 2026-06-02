# ClickHouse Topology Decision

## Purpose

Use runtime evidence and package-template evidence to recommend the hackathon MVP topology: current 1-shard package support versus a future 2 shards x 2 replicas design.

## Commands executed

```bash
kubectl exec -n upm-clickhouse-runtime clickhouse-runtime-0 -c clickhouse -- clickhouse-client --query \
  "SELECT cluster, shard_num, replica_num, host_name, port FROM system.clusters WHERE cluster='upm_cluster' ORDER BY shard_num, replica_num FORMAT Vertical"

kubectl exec -n upm-clickhouse-runtime clickhouse-runtime-0 -c clickhouse -- clickhouse-client --query \
  "CREATE DATABASE IF NOT EXISTS runtime_validation ON CLUSTER upm_cluster"

nl -ba upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl | sed -n '20,75p'
```

## Runtime evidence

`system.clusters` shows one shard with two replicas:

```text
cluster: upm_cluster
shard_num: 1
replica_num: 1
host_name: clickhouse-runtime-0.clickhouse-runtime-headless-svc.upm-clickhouse-runtime.svc.cluster.local
port: 9000

cluster: upm_cluster
shard_num: 1
replica_num: 2
host_name: clickhouse-runtime-1.clickhouse-runtime-headless-svc.upm-clickhouse-runtime.svc.cluster.local
port: 9000
```

Distributed DDL is not configured:

```text
Code: 139. DB::Exception: There is no DistributedDDL configuration in server config. (NO_ELEMENTS_IN_CONFIG)
```

ReplicatedMergeTree works when created manually on both replicas:

```text
query from clickhouse-runtime-0 -> 1 runtime-ok
query from clickhouse-runtime-1 -> 1 runtime-ok
system.replicas: is_readonly=0, is_session_expired=0, absolute_delay=0, queue_size=0
```

## Key findings

Template evidence:

```text
upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl:20 opens a single <shard>.
upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl:25-31 loops replicas from UNIT_COUNT inside that shard.
upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl:49 hardcodes <shard>01</shard>.
upm-packages/clickhouse/26.3.9.8/charts/files/clickhouseTemplate.tpl:50 sets replica from POD_NAME.
```

The current package supports one shard with multiple replicas. It does not express a 2x2 topology directly.

Two UnitSets could represent two shards only if service names, Keeper paths, macros, and cluster XML are generated differently per UnitSet. Current template does not provide these shard parameters.

## Option comparison

| Option | Description | Required Changes | Risk | Demo Value | Recommendation |
|---|---|---|---|---|---|
| Option A | Use current package-supported 1 shard x 2/3 replicas | Runtime template fixes for password, setting compatibility, logs/env; no shard model change | Low to Medium | Medium | Recommended for hackathon MVP |
| Option B | Implement 2 shards x 2 replicas by modifying package templates | Add shard index, multi-shard remote_servers, Keeper path strategy, macros, possibly distributed_ddl | Medium | High | Defer unless package changes are explicitly approved |
| Option C | Add ClickHouse-specific compose CRD later | New compose-operator API/controller and topology reconciliation | High | High | Productization path, not MVP |

## Open questions

- Should 2x2 be a hard requirement for the first demo, or can the demo show HA replication with 1 shard x 2 replicas?
- Should distributed DDL be required for MVP?
- Should shard topology be represented as multiple UnitSets or a future compose-level CRD?
- Should the first manager own package-template generation?

## Conclusion

Status: PASS for topology decision evidence.

Recommendation: use Option A for hackathon MVP: current package-supported 1 shard x 2 or 3 replicas with 3 Keeper nodes. Productization should later add either package-template topology parameters or a ClickHouse-specific compose CRD.

## Impact on future ClickHouse Spec

The Master Spec should separate MVP and future topology:

- MVP: 3 Keeper + 1 shard x 2 replicas ClickHouse Server.
- Future: 2 shards x 2 replicas, distributed DDL, generated shard macros, Keeper path design, and cluster-level topology API.
