# Day1 / Day2 Requirement Coverage Matrix

- Version: 0.4
- Date: 2026-06-02
- Status: Sealed
- Owner: zqw
- Related:
  - ../master-spec.md

## 1. Purpose

This matrix ensures that Day1 and Day2 requirements are treated as project scope.

MVP is the first executable loop. Non-MVP requirements must still map to later phases, Future Roadmap, or Non-Goals.

## 2. Priority Definitions

| Priority | Meaning |
|---|---|
| MVP / P0 | Required for first executable demo loop |
| P1 | Important enhancement after MVP |
| P2 | Productization or advanced operation capability |
| P3 | Long-term enhancement |
| Non-Goal | Explicitly excluded |

## 3. Day1 Coverage

| Requirement | MVP | Priority | Phase | Notes |
|---|---:|---:|---|---|
| Image and version management | Yes | P0 | Phase 01 | package version and image handling |
| Cluster spec configuration | Yes | P0 | Phase 01/03 | topology/resources/storage input |
| Configuration generation | Yes | P0 | Phase 01/02 | package templates and values |
| Instance deployment | Yes | P0 | Phase 01/03 | UnitSet-based deployment |
| Service startup | Yes | P0 | Phase 01 | ClickHouse/Keeper startup |
| Deployment status view | Yes | P0 | Phase 03 | UnitSet/Unit/Pod/PVC/Service status |
| Deployment result check | Yes | P0 | Phase 04 | healthcheck |
| Deployment record | Partial | P1 | Phase 03/Future | full persistence future |
| Cluster topology initialization | Yes | P0 | Phase 01/02 | MVP 1s2r, future NxM |
| Node macros | Yes | P0/P1 | Phase 01/02 | static first, dynamic later |
| Keeper connection | Yes | P0 | Phase 01 |
| Data/log/temp directory initialization | Yes | P0 | Phase 01 |
| Storage policy initialization | Partial | P1 | Phase 01/08 | basic storage first |
| Resource limits | Yes | P0 | Phase 01 |
| Admin account initialization | Yes | P0 | Phase 01 | Secret-based password |
| Default account restriction | Yes | P0 | Phase 01 | no `<no_password/>` |
| Network access control | Partial | P1 | Phase 03/Future | MVP internal service exposure |
| Day1 healthcheck | Yes | P0 | Phase 04 |
| Prometheus monitoring integration | Yes | P0 | Phase 05 |
| Backup task initialization | Partial | P1 | Phase 07 | narrow validation-object backup slice through approved Kubernetes Job path if GrpcCall remains unsupported |

## 4. Day2 Coverage

| Requirement | MVP | Priority | Phase | Notes |
|---|---:|---:|---|---|
| Continuous Prometheus metrics | Yes | P0 | Phase 05 |
| Dashboard visualization | No | P2 | Future | external Grafana, later templates |
| Monitoring rule management | No | P2 | Future |
| Storage capacity monitoring | Yes | P1 | Phase 06 |
| Table capacity statistics | Yes | P1 | Phase 06 |
| Partition capacity statistics | Yes | P1 | Phase 06 |
| Growth trend analysis | No | P2 | Phase 06/Future |
| TopN large tables | Yes | P1 | Phase 06 |
| TopN large partitions | Yes | P1 | Phase 06 |
| Temporary space monitoring | Partial | P1 | Phase 06 |
| TTL check | No | P2 | Phase 08/Future |
| Cleanup recommendations | No | P2 | Future |
| Insert QPS / rows / bytes | Partial | P1 | Phase 05/06 |
| Insert failure analysis | Partial | P1 | Phase 06 |
| Small-batch insert detection | No | P2 | Phase 06/Future |
| Write latency | No | P2 | Phase 06/Future |
| Kafka consumption monitoring | No | P3 | Future |
| Distributed write monitoring | No | P2 | Phase 02/Future |
| Write client statistics by user | No | P2 | Phase 06/Future | read-only diagnostics first |
| Write client statistics by client IP | No | P2 | Phase 06/Future | read-only diagnostics first |
| Write client statistics by target table | No | P2 | Phase 06/Future | read-only diagnostics first |
| Write quality validation by time | No | P2 | Phase 06/Future | row-count validation future |
| Write quality validation by partition | No | P2 | Phase 06/Future | row-count validation future |
| Write quality validation by table row count | No | P2 | Phase 06/Future | row-count validation future |
| Part count monitoring | Yes | P1 | Phase 06 |
| Inactive part monitoring | Yes | P1 | Phase 06 |
| Small part detection | Partial | P1 | Phase 06 |
| Merge task view | Yes | P1 | Phase 06 |
| Merge backlog judgment | Partial | P1 | Phase 06 |
| Mutation status view | Yes | P1 | Phase 06 |
| Kill mutation | No | P2 | Phase 08/Future | high-risk operation |
| OPTIMIZE control | No | P2 | Phase 08/Future | high-risk operation |
| Replica status view | Yes | P1 | Phase 06 |
| Replication delay monitoring | Yes | P1 | Phase 06 |
| Replication queue monitoring | Yes | P1 | Phase 06 |
| Abnormal replica detection | Yes | P1 | Phase 06 |
| Add replica | No | P2 | Phase 08/Future |
| Remove replica | No | P2 | Phase 08/Future |
| Keeper node status | Yes | P0/P1 | Phase 04/06 |
| Keeper leader/follower status | Yes | P0/P1 | Phase 04/06 |
| Keeper session/connection metrics | Partial | P1 | Phase 05/06 |
| Keeper latency | Partial | P1 | Phase 05/06 |
| Backup task management | Partial | P1 | Phase 07 | validation task create/status only; no retention |
| Scheduled backup | Partial | P1 | Phase 07 | API-managed Kubernetes CronJob validation slice; no retention cleanup |
| Metadata backup | Partial | P1 | Phase 07 | validation table metadata only |
| Backup retention | No | P2 | Phase 07/Future |
| Restore verification | Partial | P1 | Phase 07 | restore into validation database/table and verify row count/checksum where feasible |
| Config view/change/validation | Partial | P2 | Phase 08 |
| Config reload/rolling restart | No | P2 | Phase 08 |
| Config rollback | No | P2 | Phase 08/Future |
| Instance list/status/health | Yes | P0/P1 | Phase 03/04 |
| Rolling restart | No | P2 | Phase 08 |
| Instance offline/recovery | No | P2 | Phase 08 |
| Storage expansion | No | P2 | Phase 08 |
| Instance expansion | No | P2 | Phase 08 |
| Replica expansion | No | P2 | Phase 08 |
| Shard scale-out | No | P2 | Phase 02/08/Future |

## 5. Rule

No requirement may be silently dropped. Any new requirement must be mapped to MVP, a phase, Future, or Non-Goal.
