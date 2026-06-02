# Phase 04 MySQL Day1 Day2 Implementation Analysis

## Purpose

Analyze how current public UPMIO repositories implement MySQL deployment, topology, backup, user setup, failover-related behavior, and other operational tasks. This is evidence for future ClickHouse design, not implementation work.

## Commands Executed

```bash
grep -R "mysql" -n unit-operator compose-operator upm-packages | head -300
find unit-operator compose-operator upm-packages -iname "*mysql*" -print
grep -R "MysqlReplication\|MySQL\|ProxySQL\|backup\|restore\|user\|grant" -n unit-operator compose-operator upm-packages | head -500
rg -n "mysqldump|xtrabackup|xbcloud|CREATE USER|GRANT|MON_USER|REPL_USER|PROV_USER" unit-operator compose-operator upm-packages -S
```

## Key Findings

### Case A: Deploy MySQL HA Cluster

- HA model: public examples show primary-replica replication with `rpl_async` or `rpl_semi_sync`; there is also a separate `MysqlGroupReplication` CRD for Group Replication.
- Components involved:
  - `unit-operator`: creates and manages per-instance `Unit`/`UnitSet` resources and related Kubernetes resources.
  - `compose-operator`: configures replication topology and read/write service routing through `MysqlReplication`; handles Group Replication through `MysqlGroupReplication`; syncs ProxySQL through `ProxysqlSync`.
  - `upm-packages`: supplies MySQL images, Helm templates, init scripts, unit-agent image, and metrics sidecar configuration.
- CRDs used: `UnitSet`, `Unit`, `MysqlReplication`, optionally `MysqlGroupReplication`, optionally `ProxysqlSync`, and `GrpcCall` for day-2 tasks.
- Helm charts used: operator charts under `unit-operator/charts` and `compose-operator/charts`; MySQL package charts under `upm-packages/mysql-community/<version>/charts`.
- Single MySQL instance: `Unit`.
- Group of MySQL instances: `UnitSet`.
- MySQL replication topology: `MysqlReplication` or `MysqlGroupReplication`.
- ProxySQL: `ProxysqlSync` references ProxySQL nodes and a `MysqlReplication`.
- Pod/PVC/Service/Secret/ConfigMap:
  - `UnitSet` examples state the unit controller creates ConfigMap, headless service, external service, unit services, Units, and optional PodMonitor.
  - Secrets are provided by the human/operator manifest.
  - Pods/PVCs are implied by Unit/UnitSet reconciliation and package templates.
- Replication configuration: handled by `compose-operator` controllers and MySQL utility code against the MySQL instances.
- Failover: public code has topology/status and source/replica reconciliation behavior. Full automated failover behavior needs runtime validation and is marked not yet fully verified.
- Status: stored in CRD status fields such as `MysqlReplicationStatus.Topology`, read/write service names, readiness, conditions, and observed generation.

Manual operation path:

```bash
kubectl apply -f unit-operator/examples/unitsets/mysql-community-rpl_semi_sync-unitset.yaml
kubectl apply -f compose-operator/examples/mysql/semi-sync-replication.yaml
kubectl apply -f compose-operator/examples/proxysql/sync-mysql-backend.yaml   # if ProxySQL is used
kubectl get unitsets,units,mysqlreplications,proxysqlsyncs
```

### Case B: MySQL Backup

- Backup is implemented as `GrpcCall` to the unit-agent.
- Modeled as a CRD task, not a CronJob in the inspected examples.
- Tools:
  - logical backup uses `mysqldump`.
  - physical backup/restore paths use `xtrabackup`, `xbcloud`, and `xbstream`.
- Backup configuration is passed in `GrpcCall.spec.parameters`.
- Backup status is stored in `GrpcCall.status`.
- Object storage parameters are supported in the gRPC request path.
- Restore is supported by `restore` action.

Manual operation path:

```bash
kubectl apply -f unit-operator/examples/operations/mysql-logical-backup.yaml
kubectl describe grpccall mysql-logical-backup
kubectl get grpccall mysql-logical-backup -o yaml
```

### Case C: MySQL User Creation

- General arbitrary MySQL user creation was not found as a standalone public CRD.
- Package init scripts create required platform users such as monitor, replication, and provision users from env/secret inputs.
- ProxySQL user sync exists through `ProxysqlSync`.
- Password handling uses Kubernetes Secret references and encrypted password tooling in compose examples.
- Idempotency/audit for arbitrary user creation is not verified; status is available for `ProxysqlSync`, not for a generic MySQL user resource.

Manual operation path:

```bash
# Platform users are created during MySQL package initialization using MON_USER, REPL_USER, PROV_USER.
kubectl apply -f unit-operator/examples/unitsets/mysql-community-rpl_semi_sync-unitset.yaml

# ProxySQL user sync path:
kubectl apply -f compose-operator/examples/proxysql/sync-mysql-backend.yaml
kubectl get proxysqlsync -o yaml
```

## Evidence

MySQL UnitSet evidence from `unit-operator/examples/unitsets/mysql-community-rpl_semi_sync-unitset.yaml`:

```yaml
# UnitSet is a cluster-scoped resource that manages multiple Units
# Unit represents an individual MySQL instance
# Controller automatically creates necessary resources: ConfigMaps, Services, Pods, etc.
kind: UnitSet
spec:
  type: mysql
  edition: community
  version: "8.0"
  units: 3
  env:
    - name: ARCH_MODE
      value: rpl_semi_sync
```

MySQL replication evidence from `compose-operator/examples/mysql/semi-sync-replication.yaml`:

```yaml
kind: MysqlReplication
spec:
  mode: rpl_semi_sync
  source:
    name: mysql-semi-sync-0
    host: mysql-semi-sync-0.default.svc.cluster.local
    port: 3306
  replica:
    - name: mysql-semi-sync-1
      host: mysql-semi-sync-1.default.svc.cluster.local
      port: 3306
    - name: mysql-semi-sync-2
      host: mysql-semi-sync-2.default.svc.cluster.local
      port: 3306
```

Topology status evidence from `compose-operator/api/v1alpha1/mysqlreplication_types.go`:

```go
type MysqlReplicationStatus struct {
    Topology MysqlReplicationTopology `json:"topology"`
    ReadWriteService string `json:"readwriteService"`
    ReadOnlyService string `json:"readonlyService,omitempty"`
    Ready bool `json:"ready"`
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

Group Replication evidence from `compose-operator/examples/mysql/group-replication.yaml`:

```yaml
kind: MysqlGroupReplication
spec:
  member:
    - name: mysql-group-replication-0
    - name: mysql-group-replication-1
    - name: mysql-group-replication-2
```

ProxySQL evidence from `compose-operator/examples/proxysql/sync-mysql-backend.yaml`:

```yaml
kind: ProxysqlSync
spec:
  proxysql:
    - name: proxysql-0
      host: proxysql-0.default.svc.cluster.local
      port: 6032
  mysqlReplication: "mysql-replication-example"
```

Backup evidence from `unit-operator/examples/operations/mysql-backup.yaml`:

```yaml
kind: GrpcCall
spec:
  targetUnit: mysql-cluster-0
  type: mysql
  action: logical-backup
```

Backup tool evidence from `unit-operator/pkg/agent/app/mysql/impl.go`:

```text
mysqldump
xtrabackup
xbcloud
```

User setup evidence from `upm-packages/mysql-community/8.4.5/image/service-ctl.sh`:

```bash
echo "CREATE USER '${MON_USER}'@'%' ..."
echo "GRANT USAGE, PROCESS, REPLICATION CLIENT, REPLICATION SLAVE, SELECT ON *.* TO '${MON_USER}'@'%';"
echo "CREATE USER '${REPL_USER}'@'%' ..."
echo "GRANT REPLICATION CLIENT, REPLICATION SLAVE, SYSTEM_VARIABLES_ADMIN, REPLICATION_SLAVE_ADMIN, GROUP_REPLICATION_ADMIN, RELOAD, BACKUP_ADMIN, CLONE_ADMIN ON *.* TO '${REPL_USER}'@'%';"
echo "CREATE USER '${PROV_USER}'@'%' ..."
echo "GRANT ALL PRIVILEGES ON *.* TO '${PROV_USER}'@'%' WITH GRANT OPTION;"
```

## Required Summary

| MySQL Operation | UPMIO Component | Implementation Mechanism | Human Operation Path | Relevance to ClickHouse |
|---|---|---|---|---|
| HA deployment | unit-operator + compose-operator + upm-packages | `UnitSet` for instances, `MysqlReplication`/`MysqlGroupReplication` for topology | Apply UnitSet, then topology CRD | Use UnitSet for server/Keeper; consider topology CRD later |
| backup | unit-operator agent | `GrpcCall` to unit-agent, tools `mysqldump`/`xtrabackup`/`xbcloud` | Apply GrpcCall and inspect status | ClickHouse already has GrpcCall backup hooks |
| user creation | upm-packages init + ProxySQL sync | init SQL for platform users; `ProxysqlSync` for ProxySQL | Provide secrets/env, apply UnitSet/ProxysqlSync | Need explicit ClickHouse user story for MVP |
| failover | compose-operator | topology reconciliation and service updates; full behavior not runtime verified | Change/observe `MysqlReplication` | ClickHouse requires a different topology/failover model |
| config change | unit-operator agent | `GrpcCall` `set-variable` and config templates | Apply GrpcCall or update UnitSet config | ClickHouse already has `set-variable` handler |
| restart | unit-operator | lifecycle managed per Unit/Pod; exact restart task not fully verified | operate Unit/Pod through Kubernetes/Unit APIs | Define whether ClickHouse MVP needs explicit restart API |

## Open Questions

- Is MySQL failover intended to be fully automatic in public compose-operator, or operator-assisted?
- Is arbitrary MySQL user creation available in a private component?
- Which object storage backends are officially supported for backup in production?

## Conclusion

UPMIO MySQL Day1 is split between UnitSet deployment and compose-operator topology CRDs. Day2 operations are primarily task-based through `GrpcCall` and unit-agent implementations. User management is partially package-initialization based and not exposed as a general public user CRD.

## Impact on Future ClickHouse Spec

ClickHouse should start with UnitSet/package deployment and GrpcCall-based day-2 operations. A future ClickHouse-specific topology manager should only be added once shard/replica reconciliation requirements exceed what package templates and labels can safely express.
