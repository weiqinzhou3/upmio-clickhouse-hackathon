# Phase 02 ClickHouse Topology Rendering Evidence

This directory contains hackathon-facing evidence assets for Repository Phase 02.

Phase 02 implements package rendering for generic `N shards x M replicas`
ClickHouse topology. A 2x2 runtime claim requires real Kubernetes apply and
ClickHouse SQL validation, not only Helm rendering or server-side dry-run.

## Scope

- Render `remote_servers` from `topology.shards` and
  `topology.replicasPerShard`.
- Render a runtime-derived `remote_servers` cluster secret so Distributed table
  queries can authenticate between replicas.
- Render deterministic runtime macros from `unit-operator/unit.sn`.
- Render service-scoped `distributed_ddl` so validation DDL can use
  `ON CLUSTER`.
- Keep the single-UnitSet mapping:

```text
unit 0 -> shard01 replica01
unit 1 -> shard01 replica02
unit 2 -> shard02 replica01
unit 3 -> shard02 replica02
```

- Keep business local table and Distributed table lifecycle DBA/application
  owned. The package only renders topology primitives.

## Keeper Path Strategy

Manager-owned validation SQL and DBA examples should use this
`ReplicatedMergeTree` path strategy:

```sql
ReplicatedMergeTree(
  '/clickhouse/tables/{cluster}/{shard}/{database}/{table}',
  '{replica}'
)
```

`{cluster}`, `{shard}`, and `{replica}` are rendered as ClickHouse macros.

## Distributed DDL

The package renders a service-scoped DDL queue:

```xml
<distributed_ddl>
  <path>/clickhouse/task_queue/ddl/<unitset-name></path>
</distributed_ddl>
```

This supports validation and DBA-managed `ON CLUSTER` DDL. It does not make the
package owner of user business schemas.

## Render And Validate

```bash
helm template ch-1s2r upm-packages/clickhouse/26.3.9.8/charts \
  --values clickhouse/phase-02/values/1s2r-values.yaml >/tmp/ch-1s2r.yaml

helm template ch-2s2r upm-packages/clickhouse/26.3.9.8/charts \
  --values clickhouse/phase-02/values/2s2r-values.yaml >/tmp/ch-2s2r.yaml

helm template ch-2s3r upm-packages/clickhouse/26.3.9.8/charts \
  --values clickhouse/phase-02/values/2s3r-values.yaml >/tmp/ch-2s3r.yaml

helm template ch-4s2r upm-packages/clickhouse/26.3.9.8/charts \
  --values clickhouse/phase-02/values/4s2r-values.yaml >/tmp/ch-4s2r.yaml

python3 clickhouse/scripts/validate-phase02-rendered-topology.py \
  --rendered /tmp/ch-1s2r.yaml --shards 1 --replicas-per-shard 2

python3 clickhouse/scripts/validate-phase02-rendered-topology.py \
  --rendered /tmp/ch-2s2r.yaml --shards 2 --replicas-per-shard 2

python3 clickhouse/scripts/validate-phase02-rendered-topology.py \
  --rendered /tmp/ch-2s3r.yaml --shards 2 --replicas-per-shard 3

python3 clickhouse/scripts/validate-phase02-rendered-topology.py \
  --rendered /tmp/ch-4s2r.yaml --shards 4 --replicas-per-shard 2
```

Expected final line:

```text
PASS topology_render_validation
```

## Kubernetes Dry-run

```bash
kubectl apply --dry-run=server -f /tmp/ch-2s2r.yaml
```

Expected result: no validation error for rendered ConfigMaps and PodTemplate.

## Runtime 2x2 Validation

The ClickHouse runtime image must include the updated package
`service-ctl.sh`. In this lab cluster the image is a node-local
`localhost/upmio/clickhouse:26.3.9.8-runtime` image, so synchronize it before
creating or recreating ClickHouse Server Pods:

```bash
SSH_PASSWORD=<node-password> \
  clickhouse/scripts/sync-runtime-image-to-nodes.sh
```

This is a reproducible lab-cluster step. A production deployment should publish
the runtime image to GHCR, Harbor, or another registry reachable by every node,
then set `global.imageRegistry` / `image.repository` / `image.tag` in the package
values instead of relying on node-local images.

Runtime validation uses:

```bash
kubectl apply -f clickhouse/phase-02/manifests/00-namespace-project.yaml
kubectl apply -f clickhouse/phase-02/manifests/02-clickhouse-keeper-unitset.yaml
kubectl apply -f clickhouse/phase-02/manifests/03-clickhouse-unitset-2s2r.yaml

kubectl wait --for=jsonpath='{.status.readyUnits}'=3 \
  unitset/clickhouse-phase02-keeper \
  -n upm-clickhouse-phase02-runtime \
  --timeout=360s

kubectl wait --for=jsonpath='{.status.readyUnits}'=4 \
  unitset/clickhouse-phase02 \
  -n upm-clickhouse-phase02-runtime \
  --timeout=360s

kubectl get unitset,pods -n upm-clickhouse-phase02-runtime -o wide
```

Expected state:

```text
clickhouse-phase02-keeper   EXPECTED/CURRENT/READY = 3/3/3
clickhouse-phase02          EXPECTED/CURRENT/READY = 4/4/4
clickhouse-phase02-0..3     READY = 2/2, STATUS = Running
```

Topology check:

```bash
kubectl exec -n upm-clickhouse-phase02-runtime clickhouse-phase02-0 \
  -c clickhouse -- service-ctl.sh login --query \
  "SELECT cluster, shard_num, replica_num, host_name FROM system.clusters WHERE cluster='upm_cluster' ORDER BY shard_num, replica_num"
```

Expected result: 4 rows, representing 2 shards x 2 replicas.

Full runtime validation:

```bash
clickhouse/scripts/validate-phase02-runtime-2s2r.sh
```

Expected final line:

```text
PASS runtime_2s2r_validation
```
