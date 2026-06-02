# Phase 02 UPMIO Repository Analysis

## Purpose

Inspect the three public UPMIO repositories and summarize their purpose, structure, CRDs, charts, examples, and database coverage. This inspection is read-only.

## Commands Executed

```bash
mkdir -p upm-clickhouse
cd upm-clickhouse
git clone https://github.com/upmio/unit-operator.git
git clone https://github.com/upmio/compose-operator.git
git clone https://github.com/upmio/upm-packages.git
git -C unit-operator fetch --all --prune
git -C compose-operator fetch --all --prune
git -C upm-packages fetch --all --prune
git -C unit-operator status --short --branch
git -C compose-operator status --short --branch
git -C upm-packages status --short --branch
tree -L 3 unit-operator
tree -L 3 compose-operator
tree -L 4 upm-packages
find unit-operator -maxdepth 5 -type f | sort
find compose-operator -maxdepth 5 -type f | sort
find upm-packages -maxdepth 5 -type f | sort
rg -n "clickhouse|mysql|prometheus|ServiceMonitor|PodMonitor|Grafana|dashboard|metrics|alert" unit-operator compose-operator upm-packages -S
```

`tree` was not installed on the local Mac:

```text
/bin/sh: tree: command not found
```

`find` and `rg` were used as fallback.

## Key Findings

1. `unit-operator` manages individual database units and UnitSets. Evidence: `api/v1alpha2/unit_types.go`, `api/v1alpha2/unitset_types.go`, `pkg/controller/unit`, `pkg/controller/unitset`.
2. `compose-operator` manages cross-unit topology and composition CRDs such as MySQL replication, Redis cluster, ProxySQL sync, and PostgreSQL replication. Evidence: `compose-operator/api/v1alpha1/*_types.go`, `compose-operator/controller/*`.
3. `upm-packages` stores database package Helm charts, image build content, templates, and agent packages. Evidence: package directories such as `mysql-community`, `clickhouse`, `clickhouse-keeper`, `redis`, `postgresql`.
4. Programming language/framework: both operators are Go projects using controller-runtime/kubebuilder-style layout. Evidence: `cmd/main.go`, `api/`, `config/crd/`, `go.mod`.
5. CRD definitions are under `config/crd/bases` in both operator repos.
6. Helm charts are under `charts/unit-operator`, `charts/compose-operator`, and per-package `upm-packages/<package>/<version>/charts`.
7. Examples are under `unit-operator/examples` and `compose-operator/examples`.
8. ClickHouse is already present in `unit-operator` examples, gRPC operation routing, and `upm-packages`.
9. MySQL is present in all three repositories.
10. Monitoring integration exists through ServiceMonitor/PodMonitor and package metrics endpoints, but no Prometheus Server or Grafana installation chart was found in these repos.

## Evidence

Repository status:

```text
unit-operator: ## main...origin/main
compose-operator: ## main...origin/main
upm-packages: ## main...origin/main
```

Directory evidence:

```text
unit-operator/api/v1alpha1
unit-operator/api/v1alpha2
unit-operator/charts/unit-operator
unit-operator/config/crd/bases
unit-operator/examples/operations
unit-operator/examples/unitsets
unit-operator/pkg/controller/grpccall
unit-operator/pkg/controller/unit
unit-operator/pkg/controller/unitset

compose-operator/api/v1alpha1
compose-operator/charts/compose-operator
compose-operator/config/crd/bases
compose-operator/controller/mysqlreplication
compose-operator/controller/mysqlgroupreplication
compose-operator/controller/proxysqlsync
compose-operator/controller/rediscluster

upm-packages/clickhouse/26.3.9.8/charts
upm-packages/clickhouse-keeper/26.3.9.8/charts
upm-packages/mysql-community/8.4.5/charts
upm-packages/proxysql/2.7.3/charts
```

CRD path evidence:

```text
unit-operator/config/crd/bases/upm.syntropycloud.io_units.yaml
unit-operator/config/crd/bases/upm.syntropycloud.io_unitsets.yaml
unit-operator/config/crd/bases/upm.syntropycloud.io_grpccalls.yaml
compose-operator/config/crd/bases/upm.syntropycloud.io_mysqlreplications.yaml
compose-operator/config/crd/bases/upm.syntropycloud.io_mysqlgroupreplications.yaml
compose-operator/config/crd/bases/upm.syntropycloud.io_proxysqlsyncs.yaml
```

ClickHouse evidence:

```text
unit-operator/examples/unitsets/clickhouse-unitset.yaml: kind: UnitSet, spec.type: clickhouse, units: 2
unit-operator/examples/unitsets/clickhouse-keeper-unitset.yaml: kind: UnitSet, spec.type: clickhouse-keeper, units: 3
unit-operator/api/v1alpha1/grpccall_types.go: ClickHouseType UnitType = "clickhouse"
upm-packages/clickhouse/README.md: supported package version clickhouse/26.3.9.8
```

MySQL evidence:

```text
unit-operator/examples/unitsets/mysql-community-rpl_semi_sync-unitset.yaml
compose-operator/examples/mysql/semi-sync-replication.yaml
compose-operator/api/v1alpha1/mysqlreplication_types.go
upm-packages/mysql-community/8.4.5/charts
```

Monitoring evidence:

```text
compose-operator/config/prometheus/monitor.yaml: kind: ServiceMonitor
unit-operator/api/v1alpha2/unitset_types.go: PodMonitor PodMonitorInfo
unit-operator/pkg/controller/unitset/unitset_podmonitor.go: creates monitoring.coreos.com/v1 PodMonitor when CRD exists
upm-packages/mysql-community/8.4.5/charts/values.yaml: upmio/mysqld-exporter:0.16.0
```

## Open Questions

- Whether private/enterprise UPMIO contains additional cluster-level APIs not present in public repos.
- Whether `ServiceGroup` exists outside these public repositories.
- Whether ClickHouse package support is already considered production-supported by UPMIO or only experimental.

## Conclusion

The public UPMIO model is split into a lower-level unit lifecycle operator, a topology/composition operator, and a package repository. ClickHouse assets already exist, but the current public topology support is package/UnitSet-oriented rather than a dedicated ClickHouse cluster CRD.

## Impact on Future ClickHouse Spec

The first ClickHouse spec should reuse `UnitSet` and package templates before proposing new controllers. A higher-level manager can orchestrate existing objects and should only request compose-operator changes if shard-level topology cannot be expressed safely with package metadata.
