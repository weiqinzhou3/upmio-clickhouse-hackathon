package model

import "testing"

func TestBackupRequestValidation(t *testing.T) {
	request := BackupRequest{
		Scope:   BackupScope{Database: "upm_backup_validation", Table: "events"},
		Storage: BackupStorage{Type: StorageTypeS3, SecretRef: "clickhouse-backup-secret", Path: "backups/ch/manual"},
		Execution: BackupExecution{
			Type: ExecutionTypeKubernetesJob,
		},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("expected backup request to pass: %v", err)
	}

	request.Storage.Path = "../bad"
	if err := request.Validate(); err == nil {
		t.Fatalf("expected unsafe backup path to fail")
	}
}

func TestRestoreRequestRequiresConfirmation(t *testing.T) {
	request := RestoreRequest{
		BackupRef: "backups/ch/manual",
		Source:    BackupScope{Database: "upm_backup_validation", Table: "events"},
		Target:    RestoreTarget{Database: "restore_validation", Table: "events_restored"},
		Storage:   BackupStorage{Type: StorageTypeS3, SecretRef: "clickhouse-backup-secret", Path: "backups/ch/manual"},
		Execution: BackupExecution{Type: ExecutionTypeKubernetesJob},
		Reason:    "restore validation drill",
	}
	if err := request.Validate(); err == nil {
		t.Fatalf("expected restore without confirm to fail")
	}

	request.Confirm = true
	if err := request.Validate(); err != nil {
		t.Fatalf("expected restore request to pass: %v", err)
	}
}

func TestBackupScheduleRequestValidation(t *testing.T) {
	successful := int32(1)
	failed := int32(1)
	request := BackupScheduleRequest{
		Name:     "validation-every-minute",
		Schedule: "*/1 * * * *",
		Scope:    BackupScope{Database: "upm_backup_validation", Table: "events"},
		Storage:  BackupStorage{Type: StorageTypeS3, SecretRef: "clickhouse-backup-secret", PathPrefix: "backups/ch/scheduled"},
		Execution: BackupExecution{
			Type:                       ExecutionTypeKubernetesCronJob,
			ConcurrencyPolicy:          "Forbid",
			SuccessfulJobsHistoryLimit: &successful,
			FailedJobsHistoryLimit:     &failed,
		},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("expected schedule request to pass: %v", err)
	}

	request.Schedule = "* * *"
	if err := request.Validate(); err == nil {
		t.Fatalf("expected malformed schedule to fail")
	}
}

func TestBackupScheduleDefaults(t *testing.T) {
	request := BackupScheduleRequest{
		Name:     "validation",
		Schedule: "*/1 * * * *",
		Scope:    BackupScope{Database: "upm_backup_validation", Table: "events"},
		Storage:  BackupStorage{SecretRef: "clickhouse-backup-secret", PathPrefix: "backups/ch/scheduled"},
	}.WithDefaults()

	if request.Storage.Type != StorageTypeS3 || request.Execution.Type != ExecutionTypeKubernetesCronJob {
		t.Fatalf("defaults not applied: %#v", request)
	}
	if request.Execution.SuccessfulJobsHistoryLimit == nil || *request.Execution.SuccessfulJobsHistoryLimit != 1 {
		t.Fatalf("unexpected successful history default: %#v", request.Execution.SuccessfulJobsHistoryLimit)
	}
}
