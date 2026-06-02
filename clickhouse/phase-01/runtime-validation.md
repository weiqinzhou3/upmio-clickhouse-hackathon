# Phase 01 Runtime Upgrade Validation

- Date: 2026-06-02
- Cluster: `192.168.35.201` master, `192.168.35.202/203/204` workers
- Namespace: `upm-clickhouse-runtime`
- ClickHouse image: `localhost/upmio/clickhouse:26.3.9.8-runtime`
- Package release: `clickhouse-package` revision 3

## Result

PASS.

## Runtime Objects

```text
clickhouse-runtime          clickhouse          expected/current/ready = 2/2/2
clickhouse-runtime-keeper   clickhouse-keeper   expected/current/ready = 3/3/3
clickhouse-runtime-0        pod ready = 2/2
clickhouse-runtime-1        pod ready = 2/2
clickhouse-runtime-keeper-0 pod ready = 2/2
clickhouse-runtime-keeper-1 pod ready = 2/2
clickhouse-runtime-keeper-2 pod ready = 2/2
PodMonitor                  clickhouse-runtime-exporter-podmon created
PVCs                        all Bound with local-path
```

## Package and Config Checks

```text
package image: localhost/upmio/clickhouse:26.3.9.8-runtime
imagePullPolicy: IfNotPresent
prometheus-config=present
no_password=absent
max_concurrent_queries=absent
password_sha256_hex=present
```

## Keeper Checks

```text
clickhouse-runtime-keeper-0 ruok=imok state=follower
clickhouse-runtime-keeper-1 ruok=imok state=follower
clickhouse-runtime-keeper-2 ruok=imok state=leader
```

## Authentication Checks

```text
service_ctl_health=pass
no_password_login=rejected
SELECT 1 AS ok -> 1
```

## Topology Checks

```text
upm_cluster shard=1 replica=1 clickhouse-runtime-0.clickhouse-runtime-headless-svc.upm-clickhouse-runtime.svc.cluster.local:9000
upm_cluster shard=1 replica=2 clickhouse-runtime-1.clickhouse-runtime-headless-svc.upm-clickhouse-runtime.svc.cluster.local:9000
```

## Replication Checks

```text
clickhouse-runtime-0 runtime_validation.events rows=1 sample=runtime-ok
clickhouse-runtime-1 runtime_validation.events rows=1 sample=runtime-ok

system.replicas:
database=runtime_validation
table=events
is_readonly=0
is_session_expired=0
absolute_delay=0
queue_size=0
```

## Metrics Checks

```text
curl http://127.0.0.1:9363/metrics returned Prometheus output.
First metric family:
ClickHouse_Info{name="ClickHouse",version="26.3.9.8",version_describe="v26.3.9.8-lts",version_major="26",version_minor="3",version_patch="9"} 1
```

## Notes

- Real Secret values were generated on the remote Kubernetes node only and were
  not committed.
- The image tag is product-oriented (`26.3.9.8-runtime`), not phase-oriented.
- Keeper validation used bash `/dev/tcp` because the Keeper image does not
  include `nc`.
