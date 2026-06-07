# Phase 09: Demo Web Console

- Version: 0.1
- Date: 2026-06-07
- Status: Draft for owner decision
- Priority: P1 for hackathon demo
- Owner: zqw
- Depends on:
  - ../master-spec.md
  - ../api/upm-api-server-v1.md
  - ../manuals/full-demo-runbook.md
  - phase-03-upm-api-server.md
  - phase-04-healthcheck.md
  - phase-05-monitoring.md
  - phase-06-day2-diagnostics.md
  - phase-07-backup-restore.md

## 1. Purpose

Build a thin web console for hackathon demonstration.

The frontend must call `upm-api-server`; it must not bypass the product API by
talking directly to Kubernetes or ClickHouse.

## 2. Scope

In scope:

1. Cluster overview page.
2. Healthcheck trigger and report page.
3. Monitoring summary page with Grafana dashboard link.
4. Day2 diagnostics page.
5. Backup, restore, scheduled backup, and task status page.
6. Basic API endpoint configuration for the lab NodePort.

## 3. Non-Goals

Out of scope:

- Login/authentication.
- Kubernetes admin console.
- Table management.
- Config change.
- Version upgrade.
- Scaling.
- Business data browse/query UI.
- Replacing Grafana.

## 4. UX Boundaries

The UI must only expose implemented capabilities.

Do not show buttons for:

- online shard scale-out;
- version upgrade;
- CPU/storage expansion;
- rolling restart;
- config apply;
- table management.

Future capabilities may appear as disabled roadmap notes outside the main demo
workflow, but they must not look executable.

## 5. Acceptance Criteria

1. UI runs in Kubernetes or as a static asset served through a documented path.
2. UI can reach `upm-api-server` through the configured NodePort or service URL.
3. Cluster overview shows topology, readiness, PVC count, and service status.
4. Healthcheck can be triggered and displays PASS/WARN/FAIL checks.
5. Monitoring page displays metrics summary and links to the Grafana dashboard.
6. Diagnostics page displays category, severity, evidence, and recommendation.
7. Backup page can create backup, restore to validation target, create/delete
   schedule, and show task status.
8. Frontend validation script or Playwright check proves that all pages load and
   key API calls return expected data.

## 6. Recommended Implementation

Use a small frontend:

- Vite + React, or a static HTML/JS app if speed is more important.
- API base URL configurable through environment or runtime config.
- No new backend service unless necessary.

The frontend should be demo-focused, not a broad product UI.

## 7. Verification

Expected validation examples:

```bash
curl -fsS "${UPM_API_SERVER_URL}/api/v1/healthz" | jq .
curl -fsS "${UPM_API_SERVER_URL}/api/v1/clusters" | jq .

# If served in Kubernetes:
kubectl -n upm-system get deploy,svc upm-web-console -o wide

# Browser/Playwright validation:
# - overview page loads
# - healthcheck action returns report
# - diagnostics table renders
# - backup page can read schedule/task data
```

## 8. Risks

| Risk | Handling |
|---|---|
| UI exposes unsupported operations | Keep strict scope and disable roadmap-only items |
| CORS or NodePort access failure | Add documented API base URL and proxy/Ingress option |
| Demo becomes frontend-only | Keep scripts and backend runtime evidence as acceptance source |
| Backup/restore button mutates data unexpectedly | Use validation database/table only |
