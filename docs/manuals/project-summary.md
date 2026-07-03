# Project Summary

- Version: 0.1
- Date: 2026-06-07
- Status: Current project summary
- Scope: UPMIO ClickHouse hackathon MVP and post-MVP plan

## 1. Current Progress

The project has completed the hackathon second-stage backend, API, runtime
validation, documentation, and evidence MVP.

Phases 01 to 07 have been implemented, validated, reviewed, repaired, closed
out, and merged into `main`.

Phase 08 has been used as the planning entry point for post-MVP operations.
Phases 09 to 12 are draft plans and have not entered implementation.

Judge-facing delivery boundary:

- The current implemented and reviewable delivery is Phase 01 to Phase 07.
- Phase 08 to Phase 12 are future iteration plans derived from the first-stage
  Spec and requirement coverage analysis.
- Phase 08 to Phase 12 must not be interpreted as unfinished current MVP tasks.
- The demo should only claim capabilities that were implemented and validated
  in Phase 01 to Phase 07.

Current practical demo capability:

- deploy and validate a ClickHouse cluster through UPMIO;
- expose product operations through `upm-api-server`;
- run healthcheck;
- validate Prometheus and Grafana monitoring;
- run read-only Day2 diagnostics;
- run immediate backup, restore, scheduled backup, and task status checks.

Current unsupported production operations:

- frontend console;
- version upgrade;
- CPU/memory/storage expansion;
- online topology expansion;
- rolling restart;
- config change and rollback;
- backup retention cleanup;
- persistent operation history;
- authentication/RBAC.

## 2. Completed Phases: Phase 01 To Phase 07

| Phase | Status | Summary |
|---|---|---|
| Phase 01 | Implemented | Fixed ClickHouse and ClickHouse Keeper package runtime path so they can be deployed through UPMIO `UnitSet`. Added Secret-based password handling, Keeper runtime, ClickHouse Server runtime, and PodMonitor foundation. |
| Phase 02 | Implemented | Added static ClickHouse topology parameterization. Package rendering supports `N shards x M replicas`; runtime validation completed for `2 shards x 2 replicas + 3 Keeper`. |
| Phase 03 | Implemented | Implemented and deployed `upm-api-server` in Kubernetes. Added cluster create, list, detail, and resource inspection APIs. |
| Phase 04 | Implemented | Implemented healthcheck API. It validates Kubernetes readiness, Keeper, SQL, topology, write/read behavior, replica health, metrics endpoint, and secret leakage safety. |
| Phase 05 | Implemented | Implemented Prometheus and Grafana monitoring validation. Added metrics summary API, PodMonitor validation, and adapted Grafana dashboard validation. |
| Phase 06 | Implemented | Implemented read-only Day2 diagnostics for replicas, replication queue, parts, merges, mutations, Keeper, storage, and write quality. |
| Phase 07 | Implemented | Implemented API-driven immediate backup, restore, scheduled backup, and task status through Kubernetes Job/CronJob resources. |

## 3. Planned Phases: Phase 08 To Phase 12

| Phase | Status | Summary |
|---|---|---|
| Phase 08 | Planned / design umbrella | Defines the design boundaries for configuration change, lifecycle operations, scaling, storage expansion, approval, and audit hooks. It is too broad to implement as one coding phase. |
| Phase 09 | Planned | Demo Web Console. A thin frontend for cluster overview, healthcheck, monitoring, diagnostics, backup, restore, scheduled backup, and task status. |
| Phase 10 | Planned | Upgrade and resource management. Covers version upgrade, CPU/memory resize, PVC/storage expansion, precheck, confirmation, postcheck, and rollback/abort guidance. |
| Phase 11 | Planned | Topology expansion. Covers replica add/remove design and shard scale-out planning. Shard scale-out remains high-risk unless runtime validation proves data routing and migration behavior. |
| Phase 12 | Planned | Policy, retention, audit, and hardening. Covers backup retention cleanup, operation history, audit records, PrometheusRule lifecycle, and authentication/RBAC planning. |

## 4. Supported Operations

| Capability | Status | Phase |
|---|---|---|
| UPMIO-based ClickHouse and Keeper deployment | Supported | Phase 01 |
| Secret-based ClickHouse admin password | Supported | Phase 01 |
| Static topology rendering | Supported | Phase 02 |
| Runtime-validated `2 shards x 2 replicas + 3 Keeper` architecture | Supported | Phase 02 |
| Cluster create API | Supported | Phase 03 |
| Cluster list/detail APIs | Supported | Phase 03 |
| Kubernetes/UPMIO resource view API | Supported | Phase 03 |
| Healthcheck API | Supported | Phase 04 |
| Topology-aware write/read validation | Supported | Phase 04 |
| Replica health validation | Supported | Phase 04 |
| Metrics endpoint validation | Supported | Phase 04 |
| Prometheus metrics summary API | Supported | Phase 05 |
| PodMonitor and Prometheus target validation | Supported | Phase 05 |
| Grafana dashboard validation | Supported | Phase 05 |
| Read-only Day2 diagnostics | Supported | Phase 06 |
| Replica and replication queue diagnostics | Supported | Phase 06 |
| Part, merge, and mutation diagnostics | Supported | Phase 06 |
| Keeper diagnostics | Supported | Phase 06 |
| Storage diagnostics | Supported | Phase 06 |
| Write quality diagnostics | Supported | Phase 06 |
| Immediate backup API | Supported | Phase 07 |
| Restore API | Supported | Phase 07 |
| Scheduled backup API | Supported | Phase 07 |
| Backup/restore task status API | Supported | Phase 07 |

## 5. Unsupported Or Planned Operations

| Capability | Current Status | Planned Phase |
|---|---|---|
| Custom frontend console | Not supported | Phase 09 |
| Version upgrade / rolling upgrade | Not supported | Phase 10 |
| CPU/memory resource resize | Not supported | Phase 10 |
| PVC/storage online expansion | Not supported | Phase 10 |
| IO or StorageClass tuning | Not supported; design/precheck first | Phase 10 |
| Rolling restart | Not supported; design exists in Phase 08 | Phase 10 or a separate approved phase |
| Instance stop/start | Not supported; design exists in Phase 08 | Future phase |
| Config change and rollback | Not supported; design exists in Phase 08 | Phase 10 or future phase |
| Online replica add | Not supported | Phase 11 |
| Safe replica removal | Not supported | Phase 11 or future phase |
| Online shard scale-out | Not supported; high-risk roadmap item | Phase 11 plan-only unless approved |
| Historical data resharding / redistribution | Not supported | Future high-risk productization |
| Backup retention cleanup | Not supported | Phase 12 |
| Backup policy history | Not supported | Phase 12 |
| Persistent operation history | Not supported | Phase 12 |
| Audit records for mutating operations | Not supported | Phase 12 |
| PrometheusRule alert lifecycle | Not supported | Phase 12 or future phase |
| Authentication/RBAC | Not supported | Phase 12 or future hardening |
| Full user/role/profile/quota lifecycle | Not supported | Future phase |
| Business table management | Not supported by design | Future DBA workflow only if explicitly approved |

## 6. Architecture Scope Clarification

The current runtime-validated architecture is:

```text
2 shards x 2 replicas + 3 Keeper
```

The package rendering layer has examples and validation for:

```text
1 shard x 2 replicas
2 shards x 2 replicas
2 shards x 3 replicas
4 shards x 2 replicas
```

However, the current `upm-api-server` cluster creation flow requires the
requested topology to match the topology rendered by the installed ClickHouse
package version. Therefore, the current project should not claim arbitrary
online topology changes or arbitrary runtime architectures as supported.

Safe demo wording:

```text
The MVP validates the 2 shards x 2 replicas + 3 Keeper architecture. Other
topologies have package-rendering groundwork, but each topology must be
installed, applied, and runtime-validated before being claimed as supported.
```

## 7. Hackathon Positioning

The current project satisfies the hackathon second-stage MVP goal at the
backend/control-plane, runtime validation, documentation, and AI evidence level.

Strongest demo points:

- real Kubernetes runtime;
- UPMIO `UnitSet` and package-based orchestration;
- product API through `upm-api-server`;
- healthcheck and structured acceptance report;
- Prometheus and Grafana validation;
- Day2 diagnostics;
- backup, restore, and scheduled backup;
- phase-based AI coding, review, repair, and closeout evidence.

Biggest remaining demo improvement:

- build Phase 09, a thin frontend console, before attempting high-risk
  production operations such as upgrade or topology expansion.

## 8. Recommended Next Step

Recommended order:

1. Rehearse the full demo runbook.
2. Build Phase 09 frontend console.
3. Re-run the full backend validation chain.
4. Only then consider one extra operation feature.

If one extra operation feature is selected, prefer rolling restart or backup
retention cleanup. Do not attempt automatic shard scale-out before the
hackathon demo unless there is enough time for implementation, validation,
review, rollback testing, and closeout.
