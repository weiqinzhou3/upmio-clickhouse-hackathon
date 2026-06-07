# Day2 Diagnostics Design

- Version: 0.4
- Date: 2026-06-07
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
| Storage | Kubernetes PVC metadata | PVC binding state and requested capacity |
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

MVP implements read-only visibility first. Corrective actions are future work.

Phase 06 implements:

- write client statistics from `system.query_log` when it exists;
- `UNKNOWN` write-client finding when `system.query_log` is disabled or absent;
- optional read-only row-count validation when the API caller supplies
  `database` and `table`;
- optional `partition`, `timeColumn`, `startTime`, `endTime`, and
  `expectedRows` criteria for row-count validation.

## 4. Output Model

Diagnostics output should include:

- status: `PASS`, `WARN`, `FAIL`, or `UNKNOWN`;
- finding severity: `INFO`, `WARN`, `CRITICAL`, or `UNKNOWN`;
- summary;
- evidence;
- recommended next action;
- whether human review is required.

Status aggregation:

| Finding Severity | Report Status Impact |
|---|---|
| `CRITICAL` | `FAIL` |
| `WARN` | `WARN` if no `CRITICAL` exists |
| `UNKNOWN` | `UNKNOWN` only if no `WARN` / `CRITICAL` exists |
| `INFO` | `PASS` when all findings are `INFO` |

Thresholds are environment-specific and must be configurable through
`upm-api-server` runtime configuration.

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
