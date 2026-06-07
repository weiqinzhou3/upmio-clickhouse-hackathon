# Phase 07 Review Response

- Date: 2026-06-07
- Branch: `phase-07`
- Review artifact: `docs/review/phase-07-review.md`
- Response status: Accepted repairs completed

## Summary

Claude Code review found no blocking functional issue in the Phase 07
backup/restore implementation. Two medium findings and two low findings were
accepted and repaired before closeout.

## Repairs

| Finding | Decision | Resolution |
|---|---|---|
| `taskName` / `scheduleName` path parameters reused namespace validation | Accepted | Added `model.ValidateDNSLabel(field, value)` and used field-specific validation in task and schedule handlers |
| `BackupStorage.Validate(prefix bool)` boolean trap | Accepted | Split validation into `ValidatePath()` and `ValidatePathPrefix()` |
| Redundant NUL check in `validateText` | Accepted | Removed the redundant check; `unicode.IsControl` already covers NUL |
| Inline backup shell script lacks direct syntax test | Accepted | Added `TestBackupJobScriptBashSyntax` with `bash -n` |

## Notes

The informational findings are retained as MVP limitations:

- `collectTaskSensitiveValues` intentionally over-redacts all referenced Secret
  values with length at least four bytes. Over-redaction is preferred to
  under-redaction for task log evidence.
- `ActiveDeadlineSeconds=900` is acceptable for the validation-scale MVP. Large
  data backup policy tuning belongs to a later production backup policy phase.
- `BackoffLimit=0` is intentional for explicit operator feedback in the MVP.
  Transient failures should be retried by resubmitting the API request.

## Verification

```text
go test ./...                                                PASS
go vet ./...                                                 PASS
go build ./...                                               PASS
markdownlint "docs/**/*.md" "clickhouse/**/*.md"             PASS
gitleaks detect --source . --redact                          PASS
bash -n validate-backup-restore-runtime.sh                   PASS
kubectl apply --dry-run=server upm-api-server.yaml           PASS
upm-api-server image sync to 4 nodes                         PASS
kubectl rollout status deploy/upm-api-server                 PASS
clickhouse/phase-07/scripts/validate-backup-restore-runtime.sh PASS
```

Runtime validation result:

```text
PASS phase07_backup_restore_runtime_validation
```
