# ClickHouse Keeper UnitSet Validation

## Purpose

Validate whether ClickHouse Keeper can run through the existing UPMIO `UnitSet` package mechanism.

## Commands executed

```bash
kubectl create namespace upm-clickhouse-runtime
kubectl apply -f docs/runtime/manifests/clickhouse-keeper-unitset-runtime.yaml
kubectl get unitsets,units,pods,pvc,svc,endpoints -n upm-clickhouse-runtime -o wide
kubectl logs -n upm-clickhouse-runtime <keeper-pod> -c clickhouse-keeper --tail=80
kubectl exec -n upm-clickhouse-runtime <keeper-pod> -c clickhouse-keeper -- bash -lc 'echo ruok | nc 127.0.0.1 9181'
kubectl exec -n upm-clickhouse-runtime <keeper-pod> -c clickhouse-keeper -- bash -lc 'echo mntr | nc 127.0.0.1 9181'
```

Manifest used:

```text
docs/runtime/manifests/clickhouse-keeper-unitset-runtime.yaml
```

## Runtime evidence

Final runtime objects:

```text
unitset/clickhouse-runtime-keeper TYPE clickhouse-keeper VERSION 26.3.9.8 EXPECTED 3 CURRENT 3 READY 3
unit/clickhouse-runtime-keeper-0  Ready upm-k8s-worker03
unit/clickhouse-runtime-keeper-1  Ready upm-k8s-worker02
unit/clickhouse-runtime-keeper-2  Ready upm-k8s-worker01
pod/clickhouse-runtime-keeper-0   2/2 Running
pod/clickhouse-runtime-keeper-1   2/2 Running
pod/clickhouse-runtime-keeper-2   2/2 Running
```

PVCs:

```text
clickhouse-runtime-keeper-0-data Bound 10Gi local-path
clickhouse-runtime-keeper-1-data Bound 10Gi local-path
clickhouse-runtime-keeper-2-data Bound 10Gi local-path
```

Services:

```text
clickhouse-runtime-keeper-0-svc          9181/TCP,9234/TCP
clickhouse-runtime-keeper-1-svc          9181/TCP,9234/TCP
clickhouse-runtime-keeper-2-svc          9181/TCP,9234/TCP
clickhouse-runtime-keeper-headless-svc   9181/TCP,9234/TCP
```

Health check:

```text
clickhouse-runtime-keeper-0 ruok -> imok
clickhouse-runtime-keeper-1 ruok -> imok
clickhouse-runtime-keeper-2 ruok -> imok
```

Leader/follower evidence:

```text
clickhouse-runtime-keeper-0 mntr: zk_server_state follower
clickhouse-runtime-keeper-1 mntr: zk_server_state leader, zk_followers 2, zk_synced_followers 2
clickhouse-runtime-keeper-2 mntr: zk_server_state follower
```

Keeper logs:

```text
Processing configuration file '/var/lib/clickhouse-keeper/conf/keeper_config.xml'.
Logging information to /var/log/clickhouse-keeper/clickhouse-keeper.log
success: unit_app entered RUNNING state
```

Source evidence:

```text
upm-packages/clickhouse-keeper/26.3.9.8/charts/files/clickhouseKeeperTemplate.tpl:18-30 builds raft_configuration from UNIT_COUNT and headless service names.
```

## Key findings

Keeper can run through UPMIO `UnitSet`.

Required runtime adjustments:

- `Project` object was required so namespace service account/RBAC existed.
- runtime Secret with key names `AES_SECRET_KEY` and `default` was required by unit-agent.
- `SECRET_MOUNT=/etc/upm/secret` and `AES_SECRET_KEY` env vars had to be injected.
- `LOG_MOUNT` had to be supplied by adding an `emptyDir` at `/var/log/clickhouse-keeper`.

Failed attempts recorded:

```text
First failure: serviceaccount upm-clickhouse-runtime-serviceaccount not found.
Fix: create Project resource for namespace.

Second failure: unit-agent CrashLoopBackOff, "AES encryption key not found in environment variable AES_SECRET_KEY".
Fix: inject AES_SECRET_KEY env and mount runtime Secret.

Third failure: Keeper could not write /clickhouse-keeper.log.
Fix: add log emptyDir so LOG_MOUNT resolves to /var/log/clickhouse-keeper.
```

## Open questions

- Should package charts include required `SECRET_MOUNT`, `AES_SECRET_KEY`, and log volume defaults?
- Should the future manager create runtime credentials or require an existing Secret?
- Should Keeper status be exposed through UPMIO status instead of requiring `mntr` probes?

## Conclusion

Status: PASS after runtime manifest adjustments.

ClickHouse Keeper forms a healthy 3-node ensemble through `UnitSet`. It is not fully turnkey from the public example because runtime prerequisites and env/log settings are required.

## Impact on future ClickHouse Spec

The Spec should require a Keeper phase before ClickHouse Server phase. It should define:

- 3 Keeper Units by default.
- headless service DNS pattern.
- health checks via `ruok` and `mntr`.
- required Secret/env/log mount behavior.
- manager-visible leader/follower status if feasible.
