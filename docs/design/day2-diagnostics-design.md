# Day2 Diagnostics Design

- Version: 0.3
- Date: 2026-05-27
- Status: Sealed
- Owner: zqw
- Related:
  - ../master-spec.md
  - api-design.md

## 1. Purpose

This document defines MVP read-only Day2 diagnostics and future operation expansion.

MVP diagnostics are observational. They do not perform destructive remediation.

## 2. MVP Diagnostic Areas

| Area | Data Source | Example Checks |
|---|---|---|
| Keeper | Keeper probes, logs, SQL state | leader/follower, session health |
| Replica | `system.replicas`, `system.replication_queue` | readonly, delay, queue size |
| Parts | `system.parts` | active parts, inactive parts, TopN large tables/partitions |
| Merges | `system.merges` | running merges, long merges |
| Mutations | `system.mutations` | failed/stuck mutations |
| Storage | PVC, Prometheus, system tables | capacity pressure, TopN tables |
| Write path | system tables/logs where available | insert failures, write volume, client summary where available |
| Services | K8s Service/Endpoint | endpoint readiness |

## 3. Write-path Requirement Coverage

The following are project scope and should be mapped to Phase 06/Future:

- write client statistics by user;
- write client statistics by client IP;
- write client statistics by target table;
- write quality validation by time;
- write quality validation by partition;
- write quality validation by table row count.

MVP may implement read-only visibility first. Corrective actions are future work.

## 4. Output Model

Diagnostics output should include:

- status: `PASS`, `WARN`, `FAIL`, or `UNKNOWN`;
- severity;
- summary;
- evidence;
- recommended next action;
- whether human review is required.

## 5. Non-Goals

MVP does not:

- kill mutations;
- run OPTIMIZE;
- remove replicas;
- perform data redistribution;
- perform automatic repair;
- depend on GrpcCall.

## 6. Future Operations

Future operation workflows may include:

- backup/restore;
- kill mutation;
- optimize control;
- replica add/remove;
- storage expansion;
- configuration change and rollback;
- TTL/partition maintenance.

High-risk operations require human approval.
