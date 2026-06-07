# Post-MVP Operations Roadmap

- Version: 0.1
- Date: 2026-06-07
- Status: Draft for owner decision
- Owner: zqw
- Related:
  - ../master-spec.md
  - ../manuals/hackathon-stage2-assessment.md
  - phase-08-config-lifecycle-scaling.md

## 1. Purpose

This document maps currently unsupported operations capabilities to future
phases. It is a decision aid, not an implementation commitment.

The current completed MVP covers deployment, topology validation, healthcheck,
monitoring, Grafana validation, Day2 diagnostics, backup, restore, and scheduled
backup for the validated demo cluster.

## 2. Does Phase 08 Cover All Unsupported Capabilities?

No.

Phase 08 covers the design boundary for configuration changes, lifecycle
operations, replica/shard scaling, storage expansion, approval, and audit hooks.
It does not fully cover or implement:

- version upgrade;
- CPU/memory/IO resource change workflow;
- production backup retention and cleanup;
- persistent operation history;
- authentication/RBAC;
- monitoring rule lifecycle;
- frontend demo console;
- full data redistribution after shard scale-out.

Phase 08 is also too broad to implement safely as one coding phase. It should
remain the design umbrella and be split into smaller implementation phases.

## 3. Unsupported Capability Map

| Capability | In Phase 08 Today? | Current Recommendation | Decision Needed |
|---|---|---|---|
| Rolling restart | Yes, design only | Implement after frontend if demo time remains | Whether to include in hackathon demo |
| Instance stop/start | Yes, design only | Future, not demo-critical | Whether to keep as roadmap only |
| Config change and rollback | Yes, design only | Future or narrow read-only plan first | Which config types are safe first |
| CPU/memory resource resize | Partial | Add Phase 10 | Whether demo needs resource resizing |
| IO/storage class tuning | Partial | Add Phase 10 as design/precheck first | Whether to implement or only document |
| PVC/storage expansion | Yes, design only | Add Phase 10; validate StorageClass support first | Whether local-path can support demo expansion |
| Version upgrade | No | Add Phase 10 | Whether to attempt before hackathon |
| Replica add/remove | Yes, design only | Add Phase 11 | Whether replica add is valuable enough for demo |
| Shard scale-out | Yes, high-risk design | Add Phase 11 as future unless real validation exists | Whether to avoid before hackathon |
| Historical resharding | Explicit non-goal | Keep future only | Whether to mention as roadmap only |
| Backup retention cleanup | No | Add Phase 12 | Whether required by judges or future only |
| Backup policy history | Partial | Add Phase 12 | Whether to implement persistent history |
| Operation audit persistence | Partial hooks only | Add Phase 12 | Storage choice for audit records |
| Monitoring rule lifecycle | No | Add Phase 12 or future | Whether PrometheusRule is needed |
| Authentication/RBAC | No | Future hardening | Not recommended before demo |
| Frontend UI | No | Add Phase 09 and do next | Confirm UI scope |

## 4. Recommended Phase Split

### Phase 09: Demo Web Console

Build a thin frontend for the already implemented APIs:

- cluster overview;
- healthcheck;
- monitoring summary and Grafana link;
- diagnostics;
- backup, restore, scheduled backup, and task status.

Reason: it improves hackathon demo quality without changing the validated
backend operation path.

### Phase 10: Upgrade and Resource Management

Implement or strictly design:

- version upgrade precheck and rolling upgrade plan;
- CPU/memory resource resize plan/apply;
- PVC/storage expansion precheck and apply where StorageClass supports it;
- post-change healthcheck and rollback guidance.

Reason: these are important production operations, but they are riskier than a
frontend because they mutate runtime infrastructure.

### Phase 11: Topology Expansion

Implement or strictly design:

- replica add with post-sync validation;
- replica removal with approval-only safety boundary;
- shard scale-out plan without automatic historical data redistribution unless
  explicitly implemented and verified.

Reason: topology changes are valuable but high-risk. Shard scale-out requires
data movement, routing, and DDL ownership decisions.

### Phase 12: Policy, Retention, Audit, and Hardening

Implement or strictly design:

- backup retention cleanup;
- backup policy status;
- persistent operation history;
- PrometheusRule / alerting lifecycle if required;
- authentication/RBAC plan.

Reason: these improve production readiness but are less visible than the demo
console and less fundamental than upgrade/resource/topology workflows.

## 5. Recommended Hackathon Order

Recommended order before the hackathon demo:

1. Finish the full demo runbook and rehearse it.
2. Build the thin frontend.
3. Run the complete backend validation chain again.
4. Only then consider one additional operation feature.

If one additional operation feature is chosen, prefer rolling restart or backup
retention cleanup. Do not attempt automatic shard scale-out before the demo
unless there is enough time for full runtime validation and rollback testing.

## 6. Decision Table

| Decision | Recommended Answer | Rationale |
|---|---|---|
| Should frontend come before unsupported operations? | Yes | It improves scoring and uses validated APIs |
| Should version upgrade be attempted before demo? | No by default | High risk, requires rollback proof |
| Should CPU/storage expansion be attempted before demo? | No by default | Depends on StorageClass and UPMIO update behavior |
| Should rolling restart be implemented before demo? | Optional | Useful and bounded if prechecks are strict |
| Should replica add be implemented before demo? | Optional but lower priority | More complex than restart and less visible |
| Should shard scale-out be implemented before demo? | No | Requires data routing and resharding decisions |
| Should backup retention be implemented before demo? | Optional | Simple enough if limited to API-managed prefixes |

## 7. Acceptance Rule For Any New Operation Phase

No new operation should be claimed as supported unless it has:

1. API reference update.
2. Validation script against the real Kubernetes environment.
3. Post-operation healthcheck.
4. Failure/rollback or abort behavior.
5. Review report and accepted repair.
6. Demo-safe wording that distinguishes implemented behavior from roadmap.
