# Phase 08: Config, Lifecycle, and Scaling Management

- Version: 0.5
- Date: 2026-05-27
- Status: Future roadmap / not implemented in current MVP
- Priority: P2
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - ../evidence-summary.md
  - ../design/api-design.md
  - ../design/resource-model.md
  - ../design/day1-day2-requirement-coverage.md
  - ../architecture/clickhouse-ha-architecture.md
  - phase-02-clickhouse-topology.md
  - phase-03-upm-api-server.md
  - phase-06-day2-diagnostics.md

## 1. Purpose

Define and phase future ClickHouse configuration change, instance lifecycle, and scaling management.

This phase does not automatically execute high-risk operations by default. It creates controlled workflows and acceptance boundaries for later productization.

Delivery boundary:

- Phase 08 is not part of the implemented Phase 01-07 hackathon MVP delivery.
- It records future productionization direction for config, lifecycle, and
  scaling operations.
- Do not treat this phase as an unfinished current-delivery implementation
  requirement.
- No Phase 08 capability may be claimed as supported until a later implementation
  phase adds API, runtime validation, review, and closeout evidence.

## 2. Scope

In scope:

1. Config change and rollback design.
2. Instance lifecycle management design.
3. Replica add/remove design.
4. Shard scale-out design.
5. Storage expansion design.
6. Rolling restart workflow design.
7. Approval/audit hooks for high-risk operations.
8. API/task model for future operations.

## 3. Non-Goals

Out of scope unless separately approved:

- Automatic data redistribution.
- Fully automated resharding of historical data.
- Destructive replica removal without human approval.
- Production backup/restore implementation.
- New compose-operator CRD in MVP.
- Direct host-level operations.

## 4. Operation Categories

| Category | Example | Default Execution Policy |
|---|---|---|
| Read-only validation | precheck, status query | allowed |
| Low-risk declarative update | labels, metadata | allowed with validation |
| Restart/lifecycle | rolling restart, stop/start | requires confirmation |
| Config change | profile/quota/storage policy | requires validation and rollback plan |
| Replica change | add/remove replica | requires precheck and confirmation |
| Shard scale-out | add shard | design/future; not automatic MVP |
| Destructive action | drop data, restore overwrite | blocked unless explicitly approved |

## 5. Config Change Design

Required elements:

- Current config view.
- Desired config proposal.
- Static validation.
- Rendered diff.
- Rollout plan.
- Healthcheck after change.
- Rollback plan.
- Audit record.

Allowed first config targets:

- ClickHouse profile/setting templates.
- Prometheus endpoint setting.
- resource/storage values.

Do not implement arbitrary free-form config writes without validation.

## 6. Lifecycle Design

Required workflows:

- Rolling restart.
- Instance status view.
- Safe instance stop/start design.
- Post-restart healthcheck.

Safety rules:

- Never restart majority of Keeper nodes simultaneously.
- For ClickHouse Server, check replica state before restart.
- After each instance restart, run a reduced healthcheck.

## 7. Scaling Design

### 7.1 Replica expansion

Flow:

```text
precheck -> update topology/UnitSet -> wait for new Unit Ready -> verify replica sync -> run healthcheck -> update status
```

Required checks:

- Keeper healthy.
- Existing replicas not readonly.
- StorageClass available.
- New PVC Bound.
- New replica visible in `system.replicas`.

### 7.2 Shard scale-out

Shard scale-out is more complex and not a simple GrpcCall task.

Required design elements:

- topology value update or future topology CRD.
- remote_servers regeneration.
- macros generation.
- Distributed table/write routing strategy.
- DDL idempotency.
- migration/rebalance policy.
- user approval.

This phase must not claim automatic data redistribution unless implemented and verified.

### 7.3 Storage expansion

Required checks:

- StorageClass supports expansion.
- PVC expansion allowed.
- filesystem resize behavior understood.
- ClickHouse sees new capacity.

## 8. API Candidates

Candidate endpoints, subject to future phase confirmation:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/clusters/{namespace}/{name}/restart` | Rolling restart |
| `POST` | `/api/v1/clusters/{namespace}/{name}/config/plan` | Render config change plan |
| `POST` | `/api/v1/clusters/{namespace}/{name}/config/apply` | Apply approved config change |
| `POST` | `/api/v1/clusters/{namespace}/{name}/replicas` | Add replica |
| `POST` | `/api/v1/clusters/{namespace}/{name}/shards` | Add shard, future only |
| `POST` | `/api/v1/clusters/{namespace}/{name}/storage/expand` | Expand PVC/storage |

All mutating endpoints must support actor/reason/confirm fields.

## 9. Files Likely Changed

```text
backend/internal/api/ops_handler.go
backend/internal/model/ops.go
backend/internal/task/restart.go
backend/internal/task/config_change.go
backend/internal/task/scaling.go
backend/internal/upmio/unitset_update.go
backend/internal/clickhouse/precheck.go
docs/design/api-design.md
docs/design/resource-model.md
docs/design/day2-diagnostics-design.md
```

## 10. Acceptance Criteria

1. Each operation category has documented prechecks, execution plan, rollback/abort strategy, and postcheck.
2. Rolling restart design prevents simultaneous Keeper majority restart.
3. Replica expansion design validates new replica visibility and health.
4. Shard scale-out is explicitly marked as future/high-risk unless fully implemented.
5. Storage expansion design checks StorageClass/PVC expansion support.
6. Mutating APIs require actor, reason, and explicit confirmation fields.
7. No destructive operation is automated without approval.
8. Phase output maps config/lifecycle/scaling requirements from Day1/Day2 coverage matrix.
9. Open risks remain tracked instead of silently assumed solved.

## 11. Verification Commands

This phase is primarily design/planning unless implementation is explicitly approved.

For implemented operations:

```bash
# Backend tests
cd backend
go test ./...

# Precheck examples
curl -sS -X POST http://127.0.0.1:<port>/api/v1/clusters/<ns>/<name>/config/plan \
  -H 'Content-Type: application/json' \
  -d @examples/config-change-plan.json | jq .

# Kubernetes state checks
kubectl get unitsets,units,pods,pvc,svc -n <namespace> -o wide

# ClickHouse postchecks
kubectl exec -n <namespace> <clickhouse-pod> -c clickhouse -- clickhouse-client --query "SELECT database, table, is_readonly, is_session_expired, queue_size FROM system.replicas FORMAT Vertical"
```

If implementation is not approved, verification is documentation review plus explicit future task registration.

## 12. Risks and Open Questions

| Risk / Question | Handling |
|---|---|
| Shard expansion implies data movement | Treat as future productization unless explicit migration design exists |
| Config changes may require restart | Plan rollout and rollback before applying |
| Instance stop/start can reduce HA | Require replica/Keeper precheck |
| PVC expansion depends on StorageClass | Validate before operation |
| Approval model not fully built | Keep actor/reason/confirm fields and block high-risk automation by default |

## 13. Changelog

| Version | Date | Changes |
|---|---|---|
| 0.1 | 2026-05-27 | Initial phase draft |
| 0.2 | 2026-05-27 | Added config/lifecycle/scaling scope |
| 0.3 | 2026-05-27 | Added metadata and red-team fix structure |
| 0.4 | 2026-05-27 | Restored operation-specific workflows, safety rules, API candidates, acceptance criteria, and verification guidance |
| 0.5 | 2026-07-03 | Clarified Phase 08 as future roadmap, not part of the implemented Phase 01-07 MVP delivery |
