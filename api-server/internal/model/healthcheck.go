package model

import "time"

const (
	HealthStatusPass    = "PASS"
	HealthStatusWarn    = "WARN"
	HealthStatusFail    = "FAIL"
	HealthStatusSkipped = "SKIPPED"

	HealthSeverityCritical = "critical"
	HealthSeverityWarning  = "warning"
)

type HealthcheckReport struct {
	Namespace   string             `json:"namespace"`
	Name        string             `json:"name"`
	Status      string             `json:"status"`
	StartedAt   time.Time          `json:"startedAt"`
	CompletedAt time.Time          `json:"completedAt"`
	Summary     HealthcheckSummary `json:"summary"`
	Checks      []HealthcheckCheck `json:"checks"`
	RequestID   string             `json:"requestId,omitempty"`
}

type HealthcheckSummary struct {
	Passed   int `json:"passed"`
	Warnings int `json:"warnings"`
	Failed   int `json:"failed"`
	Skipped  int `json:"skipped"`
}

type HealthcheckCheck struct {
	Name      string         `json:"name"`
	Status    string         `json:"status"`
	Severity  string         `json:"severity"`
	Message   string         `json:"message"`
	Evidence  map[string]any `json:"evidence,omitempty"`
	StartedAt time.Time      `json:"startedAt"`
	EndedAt   time.Time      `json:"endedAt"`
}

func NewHealthcheckReport(namespace, name string) HealthcheckReport {
	startedAt := time.Now().UTC()
	return HealthcheckReport{
		Namespace: namespace,
		Name:      name,
		Status:    HealthStatusSkipped,
		StartedAt: startedAt,
		Checks:    []HealthcheckCheck{},
	}
}

func (r *HealthcheckReport) AddCheck(name, status, severity, message string, evidence map[string]any, startedAt time.Time) {
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	endedAt := time.Now().UTC()
	r.Checks = append(r.Checks, HealthcheckCheck{
		Name:      name,
		Status:    status,
		Severity:  severity,
		Message:   message,
		Evidence:  evidence,
		StartedAt: startedAt.UTC(),
		EndedAt:   endedAt,
	})
}

func (r *HealthcheckReport) Finalize() {
	summary := HealthcheckSummary{}
	overall := HealthStatusPass
	for _, check := range r.Checks {
		switch check.Status {
		case HealthStatusPass:
			summary.Passed++
		case HealthStatusWarn:
			summary.Warnings++
			if overall == HealthStatusPass {
				overall = HealthStatusWarn
			}
		case HealthStatusFail:
			summary.Failed++
			if check.Severity == HealthSeverityCritical {
				overall = HealthStatusFail
			} else if overall == HealthStatusPass {
				overall = HealthStatusWarn
			}
		default:
			summary.Skipped++
			if overall == HealthStatusPass {
				overall = HealthStatusWarn
			}
		}
	}
	if len(r.Checks) == 0 {
		overall = HealthStatusSkipped
	}
	r.Summary = summary
	r.Status = overall
	r.CompletedAt = time.Now().UTC()
}
