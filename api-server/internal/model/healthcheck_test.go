package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestHealthcheckReportAggregation(t *testing.T) {
	report := NewHealthcheckReport("upm-clickhouse", "clickhouse-phase03")
	report.AddCheck("kubernetes", HealthStatusPass, HealthSeverityCritical, "ready", nil, time.Now())
	report.AddCheck("metrics_endpoint", HealthStatusWarn, HealthSeverityWarning, "metrics endpoint is not reachable", nil, time.Now())
	report.Finalize()

	if report.Status != HealthStatusWarn {
		t.Fatalf("expected WARN status, got %s", report.Status)
	}
	if report.Summary.Passed != 1 || report.Summary.Warnings != 1 || report.Summary.Failed != 0 {
		t.Fatalf("unexpected summary: %#v", report.Summary)
	}
}

func TestHealthcheckCriticalFailureWins(t *testing.T) {
	report := NewHealthcheckReport("upm-clickhouse", "clickhouse-phase03")
	report.AddCheck("metrics_endpoint", HealthStatusWarn, HealthSeverityWarning, "metrics endpoint is not reachable", nil, time.Now())
	report.AddCheck("write_read_probe", HealthStatusFail, HealthSeverityCritical, "write/read failed", nil, time.Now())
	report.Finalize()

	if report.Status != HealthStatusFail {
		t.Fatalf("expected FAIL status, got %s", report.Status)
	}
}

func TestHealthcheckReportDoesNotRequireSecretFields(t *testing.T) {
	report := NewHealthcheckReport("upm-clickhouse", "clickhouse-phase03")
	report.AddCheck("no_secret_leakage", HealthStatusPass, HealthSeverityCritical, "no secret-like field is returned", map[string]any{
		"forbiddenPatterns": []string{"redacted"},
	}, time.Now())
	report.Finalize()

	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	lower := strings.ToLower(string(payload))
	for _, forbidden := range []string{"clickhouse_admin_password", "aes_secret_key", "password"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("report contains forbidden secret-like token %q: %s", forbidden, payload)
		}
	}
}
