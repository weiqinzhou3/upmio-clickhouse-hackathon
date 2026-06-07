# Phase 07 Review Report

- **Branch**: `phase-07`
- **Reviewed**: 2026-06-07
- **Commits**: `43878fe`, `c7c606d`
- **Scope**: Backup/Restore task model — model, API handlers, Kubernetes Job/CronJob creation, validation script

---

## Summary

Phase 07 implements backup, restore, scheduled backup, and task status APIs through Kubernetes `Job` and `CronJob` resources. The core design is solid: credentials flow through `SecretKeyRef` (never plaintext), restore requires explicit confirmation + reason, and task logs are redacted before surfacing. The validation script is thorough — covering backup → checksum → restore → diff, scheduled backup → restore, and schedule list/delete.

Below are findings organized by severity.

---

## Findings

### 1. [Medium] scheduleName path parameter validated as namespace

**File**: `api-server/internal/api/server.go:381` and `:320`

```go
scheduleName := r.PathValue("scheduleName")
if err := model.ValidateNamespace(scheduleName); err != nil {
```

The `getTask` handler (line 320) and `getBackupSchedule` / `deleteBackupSchedule` handlers (lines 381, 401) validate path parameters `taskName` and `scheduleName` with `model.ValidateNamespace`. These are Kubernetes Job/CronJob names and schedule names respectively — not namespaces. While Kubernetes DNS label rules (used by `ValidateNamespace`) are a functional superset of what's required here, the mismatch is semantically confusing and would reject valid Job/CronJob names that happen to not be valid namespaces in a stricter future validation change.

**Recommendation**: Use a dedicated name validation function (e.g., `ValidateDNSLabel`) or rely on the store layer's built-in validation. At minimum, document why namespace validation is reused here.

---

### 2. [Medium] Boolean-trap in BackupStorage.Validate(prefix bool)

**File**: `api-server/internal/model/backup.go:252`

```go
func (s BackupStorage) Validate(prefix bool) error {
```

The boolean `prefix` parameter selects between two entirely different validation paths:

- `true` → validates `PathPrefix` (required for scheduled backup)
- `false` → validates `Path` (required for manual backup/restore)

This is non-idiomatic Go. A caller reading `storage.Validate(true)` cannot tell what `true` means without inspecting the implementation.

**Recommendation**: Split into `ValidatePath()` and `ValidatePathPrefix()` or use a functional options pattern.

---

### 3. [Low] Redundant null-byte check in validateText

**File**: `api-server/internal/model/backup.go:310-312`

```go
if strings.Contains(value, "\x00") {
    return fmt.Errorf("%s must not contain NUL bytes", field)
}
```

`unicode.IsControl` on line 306 already catches `\x00` (NULL is a C0 control character), making this check redundant.

**Recommendation**: Remove the redundant check, or keep it as defense-in-depth with an explicit comment explaining why.

---

### 4. [Low] backupJobScript is a 125-line inline shell script

**File**: `api-server/internal/kube/backup.go:689-814`

The `backupJobScript()` function returns a large bash script as a Go string literal. This:

- Cannot be unit-tested in isolation
- Cannot be linted by shellcheck
- Mixes bash escaping with Go escaping (error-prone)

The validation script (`validate-backup-restore-runtime.sh`) provides integration-level coverage, but there is no direct test of the bash logic itself.

**Recommendation**: Extract the script to an embedded file (`//go:embed`) or at minimum add a unit test that shells out to `bash -n` to verify syntax.

---

### 5. [Info] `collectTaskSensitiveValues` reads all secret keys with `len >= 4`

**File**: `api-server/internal/kube/backup.go:489-492`

```go
for _, value := range secret.Data {
    if len(value) >= 4 {
        values = append(values, string(value))
    }
}
```

This collects *all* values from the referenced Secrets (storage secret, admin secret, aes-secret-key) with length ≥ 4 bytes. The intent is to redact those values from log output. However, it redacts broader content than necessary — for example, a hypothetical non-sensitive Secret key with a value ≥ 4 bytes would also be redacted from logs. This is conservative (over-redaction is better than under-redaction for security), but worth noting.

---

### 6. [Info] `ActiveDeadlineSeconds` hardcoded to 900

**File**: `api-server/internal/kube/backup.go:285`

```go
ActiveDeadlineSeconds: &activeDeadline, // int64(900)
```

Both `backupJob` (line 285) and `backupCronJob` (line 333) hardcode a 15-minute active deadline. This is reasonable for validation-size data but not configurable per-cluster or per-request. Large datasets may hit this deadline.

**Recommendation**: Consider making this configurable, or document as a known MVP limitation.

---

### 7. [Info] `BackoffLimit` is 0 — no retries

**File**: `api-server/internal/kube/backup.go:283`

```go
BackoffLimit: &backoffLimit, // int32(0)
```

Failed backup/restore Jobs never retry. This is intentional for the hackathon MVP, but means transient failures (e.g., temporary S3 unavailability) require manual re-submission.

---

## Things Done Well

1. **Credential security model**: Credentials flow exclusively through `SecretKeyRef` in the pod spec. Request bodies reference Secret *names* only. No plaintext credentials in APIs, Job specs, or (redacted) logs.

2. **Restore safety gates**: `confirm: true` + `reason` are required. Source and target must differ. The restore SQL checks the target table doesn't exist first, and restores into a fresh table with a fresh Keeper path.

3. **Validation thoroughness**: Input validation covers DNS label compliance, path traversal (`..`, `/` prefix), control characters, ClickHouse identifier rules, cron expression arity, concurrency policy enumeration, and length limits.

4. **Comprehensive runtime validation script**: The `validate-backup-restore-runtime.sh` script is 412 lines and covers the full lifecycle — MinIO setup, source data preparation, backup, checksum verification, restore with diff, scheduled backup with restore, and schedule cleanup. All outputs are saved to JSON/TSV files for audit.

5. **Label-based resource ownership**: Jobs and CronJobs are labeled with `upm.api/service-group.name`, `upm.api/backup.schedule-name`, and `upm-api-server: "true"`, enabling safe listing and deletion scoped to the correct managed cluster.

6. **Redaction in task evidence**: `collectTaskSensitiveValues` reads actual Secret values and redacts them from logs before returning them in the API response. The `logsRedacted: true` flag in evidence makes this observable to API consumers.

7. **Good test coverage for unit-testable parts**: Model validation, Job/CronJob rendering (including Secret sourcing assertions), task status derivation from Job conditions, and API handler integration tests all have coverage.

---

## Verification Checklist

- [x] Phase doc matches implementation (all APIs in §9 of the API doc are implemented)
- [x] RBAC in `upm-api-server.yaml` includes `jobs` and `cronjobs` create/delete verbs
- [x] `Store` interface (`platform/store.go`) declares all new methods
- [x] `fakeStore` in `server_test.go` implements all new interface methods
- [x] Secret values sourced only from `SecretKeyRef` in pod specs (verified in `TestBackupJobUsesSecretRefs`)
- [x] Restore requires `confirm: true` and `reason` (verified in `TestRestoreRequestRequiresConfirmation` and `TestCreateRestoreRequiresConfirm`)
- [x] Path traversal blocked by `validateSafePath` (tested in `TestBackupRequestValidation`)
- [x] No plaintext credentials in API responses or Job specs
- [ ] `backupJobScript` has no dedicated syntax/safety tests (noted as finding 4)
- [ ] `validateNamespace` reuse for schedule/task names (noted as finding 1)
