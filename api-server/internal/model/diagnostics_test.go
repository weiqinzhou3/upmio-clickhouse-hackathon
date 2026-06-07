package model

import "testing"

func TestDiagnosticsReportStatusAggregation(t *testing.T) {
	tests := []struct {
		name       string
		severities []string
		wantStatus string
	}{
		{name: "info only", severities: []string{DiagnosticSeverityInfo}, wantStatus: DiagnosticsStatusPass},
		{name: "warn wins over info", severities: []string{DiagnosticSeverityInfo, DiagnosticSeverityWarn}, wantStatus: DiagnosticsStatusWarn},
		{name: "critical wins", severities: []string{DiagnosticSeverityWarn, DiagnosticSeverityCritical}, wantStatus: DiagnosticsStatusFail},
		{name: "unknown without warn or critical", severities: []string{DiagnosticSeverityInfo, DiagnosticSeverityUnknown}, wantStatus: DiagnosticsStatusUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := NewDiagnosticsReport("upm-clickhouse", "clickhouse-demo", DiagnosticsFilter{Limit: 20}, DefaultDiagnosticsThresholds())
			for _, severity := range test.severities {
				report.AddFinding("replica", severity, "test", nil, "test", false)
			}
			report.Finalize()
			if report.Status != test.wantStatus {
				t.Fatalf("status=%s, want %s", report.Status, test.wantStatus)
			}
		})
	}
}

func TestDiagnosticsFilterValidation(t *testing.T) {
	if err := (DiagnosticsFilter{Limit: 20, Severity: DiagnosticSeverityWarn}).Validate(); err != nil {
		t.Fatalf("expected valid filter: %v", err)
	}
	if err := (DiagnosticsFilter{Limit: 101}).Validate(); err == nil {
		t.Fatalf("expected limit validation error")
	}
	if err := (DiagnosticsFilter{Limit: 20, Severity: "BAD"}).Validate(); err == nil {
		t.Fatalf("expected severity validation error")
	}
	if err := (DiagnosticsFilter{Limit: 20, StartTime: "2026-06-07 00:00:00"}).Validate(); err == nil {
		t.Fatalf("expected timeColumn validation error")
	}
}

func TestDiagnosticsSeverityFilter(t *testing.T) {
	report := NewDiagnosticsReport("upm-clickhouse", "clickhouse-demo", DiagnosticsFilter{Limit: 20, Severity: DiagnosticSeverityWarn}, DefaultDiagnosticsThresholds())
	report.AddFinding("replica", DiagnosticSeverityInfo, "info", nil, "none", false)
	report.AddFinding("parts", DiagnosticSeverityWarn, "warn", nil, "review", true)
	report.Finalize()
	if report.Status != DiagnosticsStatusWarn || len(report.Findings) != 1 || report.Findings[0].Category != "parts" {
		t.Fatalf("unexpected filtered report: %#v", report)
	}
}
