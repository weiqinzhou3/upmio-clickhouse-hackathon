package model

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

const (
	DiagnosticsStatusPass    = "PASS"
	DiagnosticsStatusWarn    = "WARN"
	DiagnosticsStatusFail    = "FAIL"
	DiagnosticsStatusUnknown = "UNKNOWN"

	DiagnosticSeverityInfo     = "INFO"
	DiagnosticSeverityWarn     = "WARN"
	DiagnosticSeverityCritical = "CRITICAL"
	DiagnosticSeverityUnknown  = "UNKNOWN"
)

type DiagnosticsFilter struct {
	Database     string `json:"database,omitempty"`
	Table        string `json:"table,omitempty"`
	Partition    string `json:"partition,omitempty"`
	TimeColumn   string `json:"timeColumn,omitempty"`
	StartTime    string `json:"startTime,omitempty"`
	EndTime      string `json:"endTime,omitempty"`
	ExpectedRows *int64 `json:"expectedRows,omitempty"`
	Severity     string `json:"severity,omitempty"`
	Limit        int    `json:"limit"`
}

type DiagnosticsThresholds struct {
	ReplicaDelayWarnSeconds     int `json:"replicaDelayWarnSeconds"`
	ReplicaDelayCriticalSeconds int `json:"replicaDelayCriticalSeconds"`
	ReplicationQueueWarn        int `json:"replicationQueueWarn"`
	ReplicationQueueCritical    int `json:"replicationQueueCritical"`
	ActivePartsPerPartitionWarn int `json:"activePartsPerPartitionWarn"`
	InactivePartsWarn           int `json:"inactivePartsWarn"`
	MergeElapsedWarnSeconds     int `json:"mergeElapsedWarnSeconds"`
	MutationAgeWarnSeconds      int `json:"mutationAgeWarnSeconds"`
}

type DiagnosticsReport struct {
	RequestID   string                `json:"requestId,omitempty"`
	Namespace   string                `json:"namespace"`
	Name        string                `json:"name"`
	Cluster     string                `json:"cluster"`
	Status      string                `json:"status"`
	GeneratedAt time.Time             `json:"generatedAt"`
	Filters     DiagnosticsFilter     `json:"filters"`
	Thresholds  DiagnosticsThresholds `json:"thresholds"`
	Summary     DiagnosticsSummary    `json:"summary"`
	Findings    []DiagnosticsFinding  `json:"findings"`
}

type DiagnosticsSummary struct {
	Info     int `json:"info"`
	Warnings int `json:"warnings"`
	Critical int `json:"critical"`
	Unknown  int `json:"unknown"`
}

type DiagnosticsFinding struct {
	Category            string         `json:"category"`
	Severity            string         `json:"severity"`
	Title               string         `json:"title"`
	Evidence            map[string]any `json:"evidence,omitempty"`
	Recommendation      string         `json:"recommendation"`
	HumanReviewRequired bool           `json:"humanReviewRequired"`
}

func DefaultDiagnosticsThresholds() DiagnosticsThresholds {
	return DiagnosticsThresholds{
		ReplicaDelayWarnSeconds:     60,
		ReplicaDelayCriticalSeconds: 300,
		ReplicationQueueWarn:        50,
		ReplicationQueueCritical:    500,
		ActivePartsPerPartitionWarn: 150,
		InactivePartsWarn:           50,
		MergeElapsedWarnSeconds:     1800,
		MutationAgeWarnSeconds:      600,
	}
}

func (t DiagnosticsThresholds) WithDefaults() DiagnosticsThresholds {
	defaults := DefaultDiagnosticsThresholds()
	if t.ReplicaDelayWarnSeconds <= 0 {
		t.ReplicaDelayWarnSeconds = defaults.ReplicaDelayWarnSeconds
	}
	if t.ReplicaDelayCriticalSeconds <= 0 {
		t.ReplicaDelayCriticalSeconds = defaults.ReplicaDelayCriticalSeconds
	}
	if t.ReplicationQueueWarn <= 0 {
		t.ReplicationQueueWarn = defaults.ReplicationQueueWarn
	}
	if t.ReplicationQueueCritical <= 0 {
		t.ReplicationQueueCritical = defaults.ReplicationQueueCritical
	}
	if t.ActivePartsPerPartitionWarn <= 0 {
		t.ActivePartsPerPartitionWarn = defaults.ActivePartsPerPartitionWarn
	}
	if t.InactivePartsWarn <= 0 {
		t.InactivePartsWarn = defaults.InactivePartsWarn
	}
	if t.MergeElapsedWarnSeconds <= 0 {
		t.MergeElapsedWarnSeconds = defaults.MergeElapsedWarnSeconds
	}
	if t.MutationAgeWarnSeconds <= 0 {
		t.MutationAgeWarnSeconds = defaults.MutationAgeWarnSeconds
	}
	return t
}

func (f DiagnosticsFilter) WithDefaults() DiagnosticsFilter {
	if f.Limit == 0 {
		f.Limit = 20
	}
	return f
}

func (f DiagnosticsFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 {
		return fmt.Errorf("limit must be between 1 and 100")
	}
	if f.Severity != "" && !validDiagnosticSeverity(f.Severity) {
		return fmt.Errorf("severity must be one of INFO, WARN, CRITICAL, UNKNOWN")
	}
	for field, value := range map[string]string{
		"database":   f.Database,
		"table":      f.Table,
		"partition":  f.Partition,
		"timeColumn": f.TimeColumn,
		"startTime":  f.StartTime,
		"endTime":    f.EndTime,
	} {
		if err := validateDiagnosticFilterValue(field, value); err != nil {
			return err
		}
	}
	if (f.StartTime != "" || f.EndTime != "") && f.TimeColumn == "" {
		return fmt.Errorf("timeColumn is required when startTime or endTime is set")
	}
	if f.ExpectedRows != nil && *f.ExpectedRows < 0 {
		return fmt.Errorf("expectedRows must be greater than or equal to 0")
	}
	return nil
}

func NewDiagnosticsReport(namespace, name string, filter DiagnosticsFilter, thresholds DiagnosticsThresholds) DiagnosticsReport {
	return DiagnosticsReport{
		Namespace:   namespace,
		Name:        name,
		Cluster:     name,
		Status:      DiagnosticsStatusUnknown,
		GeneratedAt: time.Now().UTC(),
		Filters:     filter.WithDefaults(),
		Thresholds:  thresholds.WithDefaults(),
		Findings:    []DiagnosticsFinding{},
	}
}

func (r *DiagnosticsReport) AddFinding(category, severity, title string, evidence map[string]any, recommendation string, humanReviewRequired bool) {
	r.Findings = append(r.Findings, DiagnosticsFinding{
		Category:            category,
		Severity:            severity,
		Title:               title,
		Evidence:            evidence,
		Recommendation:      recommendation,
		HumanReviewRequired: humanReviewRequired,
	})
}

func (r *DiagnosticsReport) Finalize() {
	if r.Filters.Severity != "" {
		filtered := make([]DiagnosticsFinding, 0, len(r.Findings))
		for _, finding := range r.Findings {
			if finding.Severity == r.Filters.Severity {
				filtered = append(filtered, finding)
			}
		}
		r.Findings = filtered
	}

	summary := DiagnosticsSummary{}
	status := DiagnosticsStatusPass
	for _, finding := range r.Findings {
		switch finding.Severity {
		case DiagnosticSeverityCritical:
			summary.Critical++
			status = DiagnosticsStatusFail
		case DiagnosticSeverityWarn:
			summary.Warnings++
			if status == DiagnosticsStatusPass {
				status = DiagnosticsStatusWarn
			}
		case DiagnosticSeverityUnknown:
			summary.Unknown++
			if status == DiagnosticsStatusPass {
				status = DiagnosticsStatusUnknown
			}
		default:
			summary.Info++
		}
	}
	if len(r.Findings) == 0 {
		status = DiagnosticsStatusPass
	}
	r.Summary = summary
	r.Status = status
}

func validDiagnosticSeverity(severity string) bool {
	switch severity {
	case DiagnosticSeverityInfo, DiagnosticSeverityWarn, DiagnosticSeverityCritical, DiagnosticSeverityUnknown:
		return true
	default:
		return false
	}
}

func validateDiagnosticFilterValue(field, value string) error {
	if value == "" {
		return nil
	}
	if len(value) > 256 {
		return fmt.Errorf("%s must be 256 characters or fewer", field)
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
