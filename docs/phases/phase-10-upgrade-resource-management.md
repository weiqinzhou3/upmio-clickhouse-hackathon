# Phase 10: Upgrade and Resource Management

- Version: 0.1
- Date: 2026-06-07
- Status: Draft for owner decision
- Priority: P2
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - ../api/upm-api-server-v1.md
  - phase-08-config-lifecycle-scaling.md

## 1. Purpose

Define and implement controlled workflows for ClickHouse version upgrade,
resource changes, and storage expansion.

This phase should not start implementation until the owner confirms which
operation is worth doing before the hackathon demo.

## 2. Scope

Candidate capabilities:

1. Version upgrade precheck and plan.
2. Rolling upgrade apply workflow.
3. CPU/memory resource resize plan and apply.
4. Storage expansion precheck and apply when supported by StorageClass.
5. Post-change healthcheck.
6. Abort/rollback guidance.
7. Operation task status.

## 3. Non-Goals

Out of scope unless separately approved:

- Major-version ClickHouse upgrade across incompatible versions.
- Automatic downgrade after failed upgrade.
- Host-level CPU, disk, or filesystem operations.
- Arbitrary IO scheduler tuning.
- Data migration or resharding.

## 4. Required Safety Model

Every mutating request must require:

- `actor`;
- `reason`;
- `confirm: true`;
- precheck result;
- expected target;
- postcheck result.

Do not apply a change if:

- Keeper is unhealthy;
- any replica is readonly or session-expired;
- replication queue is above threshold;
- current topology cannot be read;
- required package version is not installed;
- StorageClass does not support the requested expansion.

## 5. Candidate APIs

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/clusters/{namespace}/{name}/upgrade/plan` | Render version upgrade plan |
| `POST` | `/api/v1/clusters/{namespace}/{name}/upgrade/apply` | Apply approved rolling upgrade |
| `POST` | `/api/v1/clusters/{namespace}/{name}/resources/plan` | Render CPU/memory resource change plan |
| `POST` | `/api/v1/clusters/{namespace}/{name}/resources/apply` | Apply approved resource change |
| `POST` | `/api/v1/clusters/{namespace}/{name}/storage/expand/plan` | Validate PVC expansion support |
| `POST` | `/api/v1/clusters/{namespace}/{name}/storage/expand/apply` | Apply approved PVC expansion |

## 6. Acceptance Criteria

1. Plan APIs return current state, desired state, risks, and exact resources to
   change.
2. Apply APIs require confirmation and reject unsafe cluster state.
3. Version upgrade changes only UPMIO-managed declarative resources.
4. Resource resize updates UnitSet resources and waits for rollout.
5. Storage expansion checks StorageClass support before PVC updates.
6. Every apply workflow runs healthcheck after change.
7. Runtime validation script proves one selected operation in the real cluster.

## 7. Recommended Hackathon Decision

Do not implement this before the frontend unless the owner wants one more
operations feature after the frontend.

If one item is selected, choose CPU/memory resource resize or rolling restart
before version upgrade. Version upgrade has a larger rollback burden.
