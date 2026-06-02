# Phase Spec Red-team Repair Report

- Version: 0.1
- Date: 2026-05-27
- Status: Ready for Review
- Owner: zqw

## 1. Purpose

This report records the second red-team repair focused on the `docs/phases/*.md` files.

The red-team finding was that Phase Spec v0.3 had complete metadata but had regressed into generic templates. The phase files did not contain objective acceptance criteria, phase-specific implementation constraints, concrete verification commands, or explicit references to relevant design documents.

## 2. Repair Summary

All eight phase specs were rewritten to preserve the v0.3 metadata while restoring phase-specific execution content.

Common repaired elements:

- Phase-specific `Depends on` references to relevant architecture/design/runtime evidence documents.
- Phase-specific scope and non-goals.
- Concrete technical requirements.
- Files likely changed.
- Objective acceptance criteria.
- Concrete verification commands.
- Phase-specific risks and open questions.
- Updated changelog to Version 0.4.

## 3. Files Modified

| File | Change |
|---|---|
| `docs/phases/phase-01-upm-packages-clickhouse.md` | Restored package-specific password, metrics, config compatibility, runtime mounts, file targets, ClickHouse/Keeper acceptance criteria, and runtime verification commands. |
| `docs/phases/phase-02-clickhouse-topology.md` | Restored topology option comparison, `N shards x M replicas` requirements, Distributed table decision, write routing decision, and DDL idempotency decision. |
| `docs/phases/phase-03-manager-backend.md` | Restored create-cluster flow, API endpoint table, backend boundaries, logical models, operation history decision, and objective acceptance criteria. |
| `docs/phases/phase-04-healthcheck.md` | Restored check table, structured report model, validation DDL policy, Distributed table decision linkage, and concrete verification commands. |
| `docs/phases/phase-05-monitoring.md` | Restored monitoring boundary, metrics categories, Manager metrics API, Grafana ownership decision, and Prometheus validation commands. |
| `docs/phases/phase-06-day2-diagnostics.md` | Restored SQL/data source list, severity rules, write client statistics coverage, write quality validation coverage, and diagnostic verification commands. |
| `docs/phases/phase-07-backup-restore.md` | Restored GrpcCall runtime gate, backup/restore request models, safety controls, and runtime verification commands. |
| `docs/phases/phase-08-config-lifecycle-scaling.md` | Restored config/lifecycle/scaling operation categories, safety workflows, API candidates, and acceptance criteria. |

## 4. Red-team Item Closure

| Red-team Finding | Status | Notes |
|---|---|---|
| Phase specs regressed into generic templates | Fixed | Each phase now contains phase-specific requirements and verification commands. |
| Phase 01 lost package-specific requirements | Fixed | Password model, metrics, config compatibility, runtime mounts, acceptance criteria, and commands restored. |
| Phase 02 lost topology design options | Fixed | Option A/B/C comparison restored and expanded. |
| Phase 03 lost create-cluster flow/API/module boundaries | Fixed | 11-step flow, API table, logical models, and backend boundaries restored. |
| Phase 04 lost healthcheck checks/report model | Fixed | Required check table and JSON report model restored. |
| Phase 06 lost SQL queries/decision rules/output model | Fixed | SQL/data source matrix and decision rules restored. |
| Depends-on references downgraded | Fixed | Each phase now references the relevant design/architecture/runtime documents. |
| Master Spec Open Questions not tied to phases | Fixed | Phase 02, 03, 04, and 05 now explicitly resolve or refine relevant Open Questions. |

## 5. Remaining Notes

The Master Spec remains the sealed project constitution. Phase specs are confirmed task contracts and may still be refined before implementation, but they must not contradict the Master Spec.

## 6. Result

Result: ready for second red-team review.
