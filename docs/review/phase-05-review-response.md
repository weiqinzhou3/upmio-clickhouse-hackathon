# Phase 05 Review Response

- Date: 2026-06-07
- Review report: `docs/review/phase-05-review.md`
- Review verdict: PASS, proceed to Phase 06
- Response status: accepted repair for Phase 05 closeout

## Summary

The review found no blocking issues. The PASS verdict is accepted.

Four low-risk findings were repaired in Phase 05 closeout:

- F1: documented and centralized the `clickhouse` container-name assumption.
- F2: added direct `GetMetricsSummary` orchestration tests.
- F3: documented the canonical dashboard asset versus runtime-exported
  dashboard evidence.
- F4: documented the healthcheck probe row-count multiplier.
- F5: clarified `firstAvailableMetrics` return semantics for all-empty versus
  all-error query paths.

The Phase 04 `ClusterReady` precondition follow-up remains out of Phase 05
scope because it changes healthcheck behavior rather than monitoring behavior.

## Finding Response

| Finding | Decision | Result |
|---|---|---|
| F1 hardcoded `container="clickhouse"` | Accepted | Added `clickHouseContainerName` constant and used it in CPU/memory PromQL construction |
| F2 no direct `GetMetricsSummary` orchestration test | Accepted | Added table-driven tests for READY, missing target, CPU query error, missing PodMonitor, storage fallback success, and all-storage-empty DEGRADED behavior |
| F3 dashboard duplicated in two locations | Accepted | Documented that `clickhouse/grafana/upm-clickhouse-23285-dashboard.json` is the canonical provisioning asset and `clickhouse/phase-05/grafana-dashboard.json` is runtime API evidence |
| F4 probe row multiplier | Accepted | Added code comment explaining the 4x shard multiplier and that SQL assertions are authoritative |
| F5 `firstAvailableMetrics` empty-result semantics | Accepted | Changed helper to return `(samples, found, error)` so all-empty success is distinct from all-error failure |

## Verification

Targeted tests:

```text
go test ./internal/kube -run 'TestGetMetricsSummaryOrchestration|TestFirstAvailableMetrics' -count=1 -v
PASS
```

Full closeout gates are recorded in the final Phase 05 closeout response.
