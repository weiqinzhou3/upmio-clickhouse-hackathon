# Phase 11: Topology Expansion

- Version: 0.1
- Date: 2026-06-07
- Status: Draft for owner decision
- Priority: P2/P3
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - phase-02-clickhouse-topology.md
  - phase-08-config-lifecycle-scaling.md
  - phase-10-upgrade-resource-management.md

## 1. Purpose

Define and implement controlled topology expansion workflows.

This phase separates replica expansion from shard scale-out because they have
different risk levels.

## 2. Scope

Candidate capabilities:

1. Replica add plan.
2. Replica add apply.
3. Replica health and sync validation.
4. Replica removal design with approval-only safety boundary.
5. Shard scale-out plan.
6. Shard scale-out future design with data routing and redistribution risks.

## 3. Non-Goals

Out of scope unless separately approved:

- Automatic historical data redistribution.
- Silent Distributed table rebuild.
- Business table creation/alter/drop.
- Destructive replica removal.
- Shard removal.

## 4. Replica Expansion

Replica expansion is the safest topology operation to consider first.

Required flow:

```text
precheck -> plan UnitSet/topology change -> apply -> wait new Unit Ready -> verify system.replicas -> run healthcheck
```

Required checks:

- Keeper healthy.
- Existing replicas not readonly.
- Replication queue below threshold.
- StorageClass available.
- New PVC Bound.
- New replica appears in `system.replicas`.

## 5. Shard Scale-Out

Shard scale-out is high-risk and should remain future-only unless a complete
data strategy is implemented.

Required design elements:

- topology update;
- `remote_servers` regeneration;
- macro assignment;
- Distributed table routing;
- idempotent DDL plan;
- migration/rebalance plan;
- approval and rollback guidance.

Do not claim shard scale-out as supported until runtime validation proves that
new writes and existing data queries behave as expected.

## 6. Candidate APIs

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/clusters/{namespace}/{name}/replicas/plan` | Render replica add plan |
| `POST` | `/api/v1/clusters/{namespace}/{name}/replicas/apply` | Apply approved replica add |
| `POST` | `/api/v1/clusters/{namespace}/{name}/shards/plan` | Render shard add plan |
| `POST` | `/api/v1/clusters/{namespace}/{name}/shards/apply` | Apply shard add only if full strategy is implemented |

## 7. Acceptance Criteria

1. Replica add plan reports current topology, desired topology, risk checks, and
   expected Unit/PVC changes.
2. Replica add apply requires `actor`, `reason`, and `confirm: true`.
3. Post-apply status proves new Unit Ready, PVC Bound, and replica visible.
4. Healthcheck passes after replica expansion.
5. Shard scale-out remains plan-only unless full runtime validation exists.
6. No business tables are automatically created, altered, or dropped.

## 8. Recommended Hackathon Decision

Do not implement shard scale-out before the demo.

Replica add is optional, but the frontend has higher demo value and lower risk.
