# ClickHouse Scripts

All active ClickHouse operational scripts are stored in this directory.

Run scripts from the repository root so relative evidence paths are stable:

```bash
cd <repo-root>
```

## Image Preparation

```bash
SSH_PASSWORD=root clickhouse/scripts/sync-runtime-image-to-nodes.sh
SSH_PASSWORD=root clickhouse/scripts/sync-upm-api-server-image-to-nodes.sh
```

## Runtime Validation

Recommended order for the completed MVP:

```bash
clickhouse/scripts/validate-phase03-upm-api-server-runtime.sh
clickhouse/scripts/validate-phase04-healthcheck-runtime.sh
clickhouse/scripts/validate-phase05-monitoring-runtime.sh
clickhouse/scripts/validate-phase06-diagnostics-runtime.sh
clickhouse/scripts/validate-phase07-backup-restore-runtime.sh
```

Expected final lines:

```text
PASS phase03_upm_api_server_runtime_validation
PASS phase04_healthcheck_runtime_validation
PASS phase05_monitoring_runtime_validation
PASS phase06_diagnostics_runtime_validation
PASS phase07_backup_restore_runtime_validation
```

Full script usage, variables, and output files are documented in
[`docs/manuals/scripts-manual.md`](../../docs/manuals/scripts-manual.md).
