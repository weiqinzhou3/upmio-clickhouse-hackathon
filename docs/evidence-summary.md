# Evidence Summary

- Version: 0.5
- Date: 2026-06-02
- Status: Sealed
- Owner: zqw
- Related:
  - master-spec.md

## 1. Purpose

This document is the condensed evidence entry point for the UPMIO ClickHouse Hackathon Spec.

It is for AI coding agents and human reviewers who need the validated conclusions without opening every raw discovery/runtime file.

Raw evidence remains in:

- `docs/discovery/`
- `docs/runtime/`

## 2. Environment Evidence

- Kubernetes cluster was prepared on four RHEL 9.6 VMs.
- Topology: 1 control-plane node and 3 worker nodes.
- Kubernetes version: v1.35.5.
- Container runtime: containerd 2.1.5.
- CNI: Calico v3.30.0.
- Default StorageClass: `local-path`.
- Helm installed.
- Registry access from VMs is unreliable; image preloading or private registry is required.

## 3. UPMIO Model Evidence

Validated public UPMIO model:

| Component | Responsibility |
|---|---|
| `unit-operator` | Reconciles `Project`, `UnitSet`, `Unit`, and `GrpcCall` |
| `compose-operator` | Reconciles topology CRDs such as MySQL replication, Redis cluster, ProxySQL sync |
| `upm-packages` | Provides package charts, images, templates, scripts, and unit-agent integration |

Runtime evidence confirmed that UPMIO operators can be installed and that CRDs/API resources become available.

## 4. MySQL Reference Evidence

Current MySQL implementation provides an architectural reference:

- Day1 deployment uses `UnitSet` for instance lifecycle.
- MySQL replication topology uses `MysqlReplication` or `MysqlGroupReplication`.
- ProxySQL synchronization uses `ProxysqlSync`.
- Day2 tasks such as backup/restore use `GrpcCall` and unit-agent.
- General arbitrary MySQL user management CRD was not found in public repos.

## 5. ClickHouse Runtime Evidence

Validated:

- ClickHouse Keeper can run through `UnitSet(type=clickhouse-keeper)`.
- 3-node Keeper ensemble formed leader/follower state.
- ClickHouse Server can run through `UnitSet(type=clickhouse)` after package template/runtime fixes.
- `1 shard x 2 replicas` was validated as the first repeatable executable baseline.
- ReplicatedMergeTree can connect to Keeper and show healthy replica status.

Not validated / not ready:

- Current package is not turnkey.
- Current package does not directly support `2 shards x 2 replicas`.
- ClickHouse metrics endpoint is not enabled by default.
- Installed `unit-operator:v1.1.0` rejects `GrpcCall(type=clickhouse)`.

## 6. Monitoring Evidence

- UPMIO public repos provide monitoring integration points, not a full monitoring stack.
- Prometheus/Grafana are external dependencies.
- `UnitSet.spec.podMonitor` can create PodMonitor when Prometheus Operator CRDs exist.
- ClickHouse metrics endpoint was not listening in runtime validation.
- MVP should enable ClickHouse native Prometheus endpoint, not add an exporter sidecar by default.

## 7. MVP Impact

MVP should focus on:

- package fixes;
- 3 Keeper + 1 shard x 2 replicas;
- Secret-based password initialization;
- native Prometheus endpoint;
- PodMonitor integration;
- Go `upm-api-server`;
- Day1 healthcheck;
- read-only Day2 diagnostics;
- narrow backup/restore validation slice through an approved Kubernetes Job path when ClickHouse GrpcCall remains unsupported.

MVP should not depend on ClickHouse GrpcCall, full production backup/restore,
automatic shard scale-out, or a new compose-operator CRD.

## 8. Evidence Reading Rule

AI coding agents should start from:

1. `docs/master-spec.md`
2. `docs/evidence-summary.md`
3. current phase spec
4. referenced design/architecture docs

Open raw discovery/runtime files only when required by the current phase or when implementation contradicts summarized evidence.
