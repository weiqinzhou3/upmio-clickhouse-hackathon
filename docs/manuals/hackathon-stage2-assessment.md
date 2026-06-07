# Hackathon Stage 2 Assessment

## Source Of Truth

The hackathon second stage is the competition development and Demo stage. The
source material is `exam/BSG AI编程黑客松活动介绍.pdf`.

The stated second-stage deliverables are:

1. ClickHouse operations management software prototype or MVP.
2. Core code and necessary engineering notes.
3. Demo runtime environment and deployment guide.
4. Key function demo scripts or demo material.
5. Requirement coverage explanation and future evolution suggestions.

The review focus is:

- requirement understanding;
- architecture design;
- Spec quality;
- engineering completeness;
- AI usage effect;
- demo and innovation.

## Current MVP Coverage

| Requirement Area | Current Status | Evidence |
|---|---|---|
| UPMIO Operator / CRD usage | Covered | `upm-api-server` creates/reads UPMIO `Project` and `UnitSet`; manifests and runtime evidence under `clickhouse/phase-03/` |
| Standardized ClickHouse deployment | Covered | 2-shard x 2-replica ClickHouse + 3 Keeper runtime validated |
| Cluster topology initialization | Covered | `system.clusters`, macros, Keeper, distributed write/read validation |
| Product API control plane | Covered | `api-server/`, `docs/api/upm-api-server-v1.md` |
| Healthcheck / online acceptance | Covered | `POST /healthcheck`, Phase 04 validation |
| Prometheus metrics | Covered | ClickHouse `/metrics`, PodMonitor, Prometheus target validation |
| Grafana dashboard | Covered | Adapted dashboard asset and panel-query validation |
| Day2 diagnostics | Covered | Read-only diagnostics for replicas, queue, parts, merges, mutations, Keeper, storage, write stats, write quality |
| Backup | Covered | API-created Kubernetes Job and S3/MinIO validation |
| Restore | Covered | API-created Kubernetes Job, explicit confirmation, fresh target validation table |
| Scheduled backup | Covered | API-created Kubernetes CronJob and scheduled backup restore validation |
| AI usage evidence | Covered | `docs/ai-usage-phase02.md` and `docs/review/` |
| Deployment guide | Covered by this closeout | `docs/manuals/deployment-manual.md` |
| Demo scripts/materials | Covered by this closeout | `docs/manuals/demo-guide.md`, `clickhouse/scripts/` |

## Conclusion

The current repository satisfies the hackathon second-stage MVP target at the
backend/control-plane, runtime validation, documentation, and AI evidence level.

The strongest scoring points are:

- real Kubernetes runtime instead of paper-only design;
- UPMIO `UnitSet`/package-based ClickHouse orchestration;
- API-based product control plane instead of local binary operation;
- Prometheus and Grafana end-to-end validation;
- backup, restore, and scheduled backup through Kubernetes task resources;
- review-driven AI coding workflow with evidence.

## Remaining Gaps

| Gap | Impact | Recommendation |
|---|---|---|
| No custom frontend UI | Not a functional blocker, but weakens demo immediacy and product feel | Build a thin web UI if time remains |
| No production authentication/RBAC | Acceptable for isolated hackathon lab; not production-ready | Document as future work |
| No persistent operation history database | Current task status is derived from K8s resources | Add persistence later for audit/history |
| No backup retention cleanup | Explicitly out of MVP scope | Add retention policy in a future phase |
| No config lifecycle/scaling implementation | Phase 08 not implemented | Treat as roadmap/future evolution |
| kube-prometheus-stack install automation is external | Acceptable if deployment manual is clear | Add a one-command bootstrap script after demo UI or if judges require it |

## Frontend Judgment

A frontend is not strictly required by the written second-stage deliverables
because the deliverables allow a prototype/MVP with core code, API, deployment
guide, and demo scripts.

However, a frontend is likely to improve scoring in the "demo and innovation"
dimension because the activity description explicitly mentions product UI as one
possible upper-layer software form. A UI also makes the result easier for judges
to understand in a short presentation.

Recommended frontend scope if time remains:

1. Cluster overview: topology, pod readiness, storage, metrics status.
2. Healthcheck page: run healthcheck and display PASS/WARN/FAIL checks.
3. Diagnostics page: show Day2 findings by severity.
4. Backup/restore page: create backup, restore to validation target, create or
   delete backup schedule, read task status.

Avoid a large frontend. A thin API-driven UI is enough for the hackathon demo
and should not replace the already validated backend runtime path.

## Suggested Final Demo Claim

This project implements a Kubernetes-native ClickHouse operations management MVP
on top of UPMIO. It supports Day1 deployment validation, healthcheck,
Prometheus/Grafana observability, Day2 diagnostics, immediate backup, restore,
and scheduled backup through a product API server. Every major claim is backed
by runtime scripts, saved evidence, code review artifacts, and AI usage records.
