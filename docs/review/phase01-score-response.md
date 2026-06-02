# Phase 01 Score Response and Repair Plan

- Version: 0.1
- Date: 2026-06-02
- Status: Confirmed
- Owner: zqw
- Related:
  - ../ai-usage.md
  - ../ai-usage-phase02.md
  - ../master-spec.md
  - ../design/day1-day2-requirement-coverage.md
  - ../phases/phase-05-monitoring.md
  - ../phases/phase-07-backup-restore.md

## 1. Purpose

This document records the response to the hackathon Phase 01 review comments for
zqw / Zhou Qinwei and the pre-development repair items before the hackathon
second stage.

It is a review response and repair log. It is not a new product spec and does
not override `docs/master-spec.md`.

## 2. Review Summary

The Phase 01 review recognized the submission as strong overall:

- evidence-based architecture design;
- AI-consumable documentation structure;
- strong phase decomposition;
- red-team repair process;
- runtime validation on a real Kubernetes environment.

The review also identified two practical gaps that affect hackathon second-stage
demo readiness:

1. ClickHouse `GrpcCall` support was honestly recorded as unsupported, but the
   backup/restore fallback path was not concrete enough.
2. Monitoring endpoint and Prometheus scrape integration were not validated as a
   full end-to-end chain.

## 3. Repair Decisions

| Finding | Decision | Repair Target |
|---|---|---|
| `GrpcCall(type=clickhouse)` unsupported | Keep runtime evidence; do not depend on GrpcCall as the only task path | `phase-07-backup-restore.md` |
| Backup/restore is operationally important | Backup/restore remains required scope; use an approved Kubernetes Job task path for MVP/demo if GrpcCall remains unsupported | `master-spec.md`, `day1-day2-requirement-coverage.md`, `phase-07-backup-restore.md` |
| Monitoring validation incomplete | Validate the chain from ClickHouse native `/metrics` through PodMonitor and Prometheus target discovery to Manager summary | `phase-05-monitoring.md` |
| AI usage evidence will matter more in coding stage | Start a stage-two AI evidence log with Codex implementation, Claude Code review, ChatGPT discussion, and human decisions | `ai-usage-phase02.md` |

## 4. Backup/Restore Repair

The repair does not remove backup/restore from the project. It separates two
concerns:

- Backup/restore is a required database operations capability.
- The currently installed `unit-operator:v1.1.0` cannot be treated as evidence
  that ClickHouse GrpcCall works.

The repaired strategy is:

1. Record a Master Spec amendment that keeps full production backup/restore out
   of MVP scope while adding a narrow validation-object backup/restore slice.
2. Use Manager-created Kubernetes `Job` resources as the primary approved
   backup/restore task path for hackathon MVP/demo coverage.
3. Inject all ClickHouse and backup storage credentials through Kubernetes
   Secrets.
4. Restore only into validation database/table targets by default.
5. Verify restore by row count and, where feasible, checksum.
6. Keep GrpcCall source-code/image compatibility investigation as a timeboxed
   spike, not as the critical path for demo backup/restore.

## 5. Monitoring Repair

The repaired monitoring acceptance path is:

```text
ClickHouse native /metrics endpoint
  -> metrics port exposed on ClickHouse Pod/Service
  -> UPMIO UnitSet PodMonitor
  -> Prometheus target discovery
  -> Manager /metrics/summary API
```

Kubernetes `metrics-server` is useful resource evidence but is not sufficient
for ClickHouse monitoring acceptance because it does not scrape ClickHouse
native metrics and does not prove Prometheus target discovery.

If kube-prometheus-stack is unavailable in the environment, the skipped item must
be recorded with a concrete reason. Endpoint and PodMonitor validation should
still run.

## 6. Stage-Two AI Evidence Repair

The hackathon second stage will maintain `docs/ai-usage-phase02.md` as the
working evidence log.

Expected evidence sources:

- Codex implementation notes, commands, tests, and runtime evidence.
- Claude Code stage reviews and review disposition.
- ChatGPT discussion summaries provided by the human owner.
- Human decisions that accept, reject, or modify AI recommendations.

The evidence log should record outcomes and decisions, not raw full chat logs by
default.

## 7. Acceptance for This Repair

This pre-development repair is complete when:

1. Master Spec and the coverage matrix record the narrow backup/restore
   validation slice without promising a full production backup/restore loop.
2. Phase 07 documents Kubernetes Job backup/restore as the primary non-GrpcCall
   execution path.
3. Phase 07 keeps GrpcCall repair as a verified secondary spike.
4. Phase 05 documents the full monitoring chain and the limited role of
   `metrics-server`.
5. `docs/ai-usage-phase02.md` exists and is ready to receive stage-two evidence.
6. No runtime success is claimed without command evidence.

## 8. Known Risks

| Risk | Handling |
|---|---|
| Kubernetes Job path may be viewed as bypassing UPMIO task capabilities | Keep it Manager-created, SecretRef-based, namespace-scoped, and explicitly documented as an approved alternative task path |
| Native ClickHouse backup may require object storage preparation | Use MinIO/object storage if available, or record storage backend limitation with evidence |
| Prometheus stack may be blocked by image availability | Validate endpoint and PodMonitor; record full scrape as environment-blocked only when justified |
| AI evidence may become too verbose | Record concise evidence summaries, decisions, and artifact links |
