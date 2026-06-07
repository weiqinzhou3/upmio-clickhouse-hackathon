package model

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
)

const (
	TaskTypeBackup  = "backup"
	TaskTypeRestore = "restore"

	TaskStatusPending   = "Pending"
	TaskStatusRunning   = "Running"
	TaskStatusSucceeded = "Succeeded"
	TaskStatusFailed    = "Failed"
	TaskStatusUnknown   = "Unknown"

	StorageTypeS3 = "s3"

	ExecutionTypeKubernetesJob     = "kubernetesJob"
	ExecutionTypeKubernetesCronJob = "kubernetesCronJob"
)

type BackupScope struct {
	Database string `json:"database"`
	Table    string `json:"table"`
}

type BackupStorage struct {
	Type       string `json:"type"`
	SecretRef  string `json:"secretRef"`
	Path       string `json:"path,omitempty"`
	PathPrefix string `json:"pathPrefix,omitempty"`
}

type BackupExecution struct {
	Type                       string `json:"type,omitempty"`
	ConcurrencyPolicy          string `json:"concurrencyPolicy,omitempty"`
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`
	FailedJobsHistoryLimit     *int32 `json:"failedJobsHistoryLimit,omitempty"`
}

type BackupRequest struct {
	Scope     BackupScope     `json:"scope"`
	Storage   BackupStorage   `json:"storage"`
	Execution BackupExecution `json:"execution,omitempty"`
	DryRun    bool            `json:"dryRun,omitempty"`
}

type RestoreTarget struct {
	Database string `json:"database"`
	Table    string `json:"table"`
}

type RestoreRequest struct {
	BackupRef string          `json:"backupRef"`
	Source    BackupScope     `json:"source"`
	Target    RestoreTarget   `json:"target"`
	Storage   BackupStorage   `json:"storage"`
	Execution BackupExecution `json:"execution,omitempty"`
	Confirm   bool            `json:"confirm"`
	Reason    string          `json:"reason"`
	DryRun    bool            `json:"dryRun,omitempty"`
}

type BackupScheduleRequest struct {
	Name      string          `json:"name"`
	Schedule  string          `json:"schedule"`
	TimeZone  string          `json:"timeZone,omitempty"`
	Scope     BackupScope     `json:"scope"`
	Storage   BackupStorage   `json:"storage"`
	Execution BackupExecution `json:"execution,omitempty"`
	Suspend   bool            `json:"suspend,omitempty"`
}

type TaskRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type TaskStatus struct {
	RequestID      string         `json:"requestId,omitempty"`
	Name           string         `json:"name"`
	Namespace      string         `json:"namespace"`
	Cluster        string         `json:"cluster"`
	Type           string         `json:"type"`
	Status         string         `json:"status"`
	Message        string         `json:"message,omitempty"`
	StartTime      *time.Time     `json:"startTime,omitempty"`
	CompletionTime *time.Time     `json:"completionTime,omitempty"`
	JobRef         *TaskRef       `json:"jobRef,omitempty"`
	PodRef         *TaskRef       `json:"podRef,omitempty"`
	Evidence       map[string]any `json:"evidence,omitempty"`
}

type BackupScheduleStatus struct {
	RequestID            string       `json:"requestId,omitempty"`
	Name                 string       `json:"name"`
	Namespace            string       `json:"namespace"`
	Cluster              string       `json:"cluster"`
	Schedule             string       `json:"schedule"`
	TimeZone             string       `json:"timeZone,omitempty"`
	Suspend              bool         `json:"suspend"`
	ActiveJobs           []TaskRef    `json:"activeJobs"`
	LastScheduleTime     *time.Time   `json:"lastScheduleTime,omitempty"`
	LastSuccessfulTime   *time.Time   `json:"lastSuccessfulTime,omitempty"`
	RecentJobs           []TaskStatus `json:"recentJobs"`
	StorageSecretRef     string       `json:"storageSecretRef"`
	BackupPathPrefix     string       `json:"backupPathPrefix"`
	SuccessfulJobsRetain int32        `json:"successfulJobsHistoryLimit"`
	FailedJobsRetain     int32        `json:"failedJobsHistoryLimit"`
}

type BackupScheduleList struct {
	Items []BackupScheduleStatus `json:"items"`
}

func (r BackupRequest) WithDefaults(cluster string) BackupRequest {
	r.Execution.Type = defaultString(r.Execution.Type, ExecutionTypeKubernetesJob)
	r.Storage.Type = defaultString(r.Storage.Type, StorageTypeS3)
	if r.Storage.Path == "" {
		r.Storage.Path = fmt.Sprintf("backups/%s/manual-%s", cluster, time.Now().UTC().Format("20060102T150405Z"))
	}
	return r
}

func (r BackupRequest) Validate() error {
	if err := r.Scope.Validate("scope"); err != nil {
		return err
	}
	if err := r.Storage.Validate(false); err != nil {
		return err
	}
	if r.Execution.Type != ExecutionTypeKubernetesJob {
		return fmt.Errorf("execution.type must be %s", ExecutionTypeKubernetesJob)
	}
	return nil
}

func (r RestoreRequest) WithDefaults() RestoreRequest {
	r.Execution.Type = defaultString(r.Execution.Type, ExecutionTypeKubernetesJob)
	r.Storage.Type = defaultString(r.Storage.Type, StorageTypeS3)
	if r.Storage.Path == "" {
		r.Storage.Path = r.BackupRef
	}
	return r
}

func (r RestoreRequest) Validate() error {
	if strings.TrimSpace(r.BackupRef) == "" {
		return fmt.Errorf("backupRef is required")
	}
	if err := validateSafePath("backupRef", r.BackupRef); err != nil {
		return err
	}
	if err := r.Source.Validate("source"); err != nil {
		return err
	}
	if err := r.Target.Validate("target"); err != nil {
		return err
	}
	if r.Source.Database == r.Target.Database && r.Source.Table == r.Target.Table {
		return fmt.Errorf("target must be different from source")
	}
	if err := r.Storage.Validate(false); err != nil {
		return err
	}
	if r.Execution.Type != ExecutionTypeKubernetesJob {
		return fmt.Errorf("execution.type must be %s", ExecutionTypeKubernetesJob)
	}
	if !r.Confirm {
		return fmt.Errorf("confirm must be true for restore")
	}
	if strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("reason is required for restore")
	}
	return validateText("reason", r.Reason, 512)
}

func (r BackupScheduleRequest) WithDefaults() BackupScheduleRequest {
	r.Execution.Type = defaultString(r.Execution.Type, ExecutionTypeKubernetesCronJob)
	r.Execution.ConcurrencyPolicy = defaultString(r.Execution.ConcurrencyPolicy, "Forbid")
	if r.Execution.SuccessfulJobsHistoryLimit == nil {
		value := int32(1)
		r.Execution.SuccessfulJobsHistoryLimit = &value
	}
	if r.Execution.FailedJobsHistoryLimit == nil {
		value := int32(1)
		r.Execution.FailedJobsHistoryLimit = &value
	}
	r.Storage.Type = defaultString(r.Storage.Type, StorageTypeS3)
	return r
}

func (r BackupScheduleRequest) Validate() error {
	if errs := k8svalidation.IsDNS1123Label(r.Name); len(errs) > 0 {
		return fmt.Errorf("name: %s", errs[0])
	}
	if strings.TrimSpace(r.Schedule) == "" {
		return fmt.Errorf("schedule is required")
	}
	if err := validateText("schedule", r.Schedule, 128); err != nil {
		return err
	}
	if len(strings.Fields(r.Schedule)) != 5 {
		return fmt.Errorf("schedule must be a five-field Kubernetes cron expression")
	}
	if err := validateText("timeZone", r.TimeZone, 64); err != nil {
		return err
	}
	if err := r.Scope.Validate("scope"); err != nil {
		return err
	}
	if err := r.Storage.Validate(true); err != nil {
		return err
	}
	if r.Execution.Type != ExecutionTypeKubernetesCronJob {
		return fmt.Errorf("execution.type must be %s", ExecutionTypeKubernetesCronJob)
	}
	switch r.Execution.ConcurrencyPolicy {
	case "Allow", "Forbid", "Replace":
	default:
		return fmt.Errorf("execution.concurrencyPolicy must be Allow, Forbid, or Replace")
	}
	if r.Execution.SuccessfulJobsHistoryLimit == nil || *r.Execution.SuccessfulJobsHistoryLimit < 0 {
		return fmt.Errorf("execution.successfulJobsHistoryLimit must be greater than or equal to 0")
	}
	if r.Execution.FailedJobsHistoryLimit == nil || *r.Execution.FailedJobsHistoryLimit < 0 {
		return fmt.Errorf("execution.failedJobsHistoryLimit must be greater than or equal to 0")
	}
	return nil
}

func (s BackupScope) Validate(prefix string) error {
	if err := validateClickHouseIdentifier(prefix+".database", s.Database); err != nil {
		return err
	}
	return validateClickHouseIdentifier(prefix+".table", s.Table)
}

func (t RestoreTarget) Validate(prefix string) error {
	if err := validateClickHouseIdentifier(prefix+".database", t.Database); err != nil {
		return err
	}
	return validateClickHouseIdentifier(prefix+".table", t.Table)
}

func (s BackupStorage) Validate(prefix bool) error {
	if s.Type != StorageTypeS3 {
		return fmt.Errorf("storage.type must be %s", StorageTypeS3)
	}
	if errs := k8svalidation.IsDNS1123Subdomain(s.SecretRef); len(errs) > 0 {
		return fmt.Errorf("storage.secretRef: %s", errs[0])
	}
	if prefix {
		if strings.TrimSpace(s.PathPrefix) == "" {
			return fmt.Errorf("storage.pathPrefix is required")
		}
		return validateSafePath("storage.pathPrefix", s.PathPrefix)
	}
	if strings.TrimSpace(s.Path) == "" {
		return fmt.Errorf("storage.path is required")
	}
	return validateSafePath("storage.path", s.Path)
}

func validateClickHouseIdentifier(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(value) > 63 {
		return fmt.Errorf("%s must be 63 characters or fewer", field)
	}
	for index, char := range value {
		if char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return fmt.Errorf("%s must match [A-Za-z_][A-Za-z0-9_]*", field)
	}
	return nil
}

func validateSafePath(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(value) > 512 {
		return fmt.Errorf("%s must be 512 characters or fewer", field)
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, "..") {
		return fmt.Errorf("%s must be a relative object path without '..'", field)
	}
	return validateText(field, value, 512)
}

func validateText(field, value string, maxLength int) error {
	if len(value) > maxLength {
		return fmt.Errorf("%s must be %d characters or fewer", field, maxLength)
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return fmt.Errorf("%s must not contain control characters", field)
		}
	}
	if strings.Contains(value, "\x00") {
		return fmt.Errorf("%s must not contain NUL bytes", field)
	}
	return nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
