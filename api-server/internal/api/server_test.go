package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/model"
)

type fakeStore struct {
	clusters           []model.ClusterSummary
	resources          model.ClusterResources
	createErr          error
	healthErr          error
	healthDelay        time.Duration
	metricsSummary     model.MetricsSummary
	metricsErr         error
	diagnosticsReport  model.DiagnosticsReport
	diagnosticsErr     error
	taskStatus         model.TaskStatus
	backupErr          error
	restoreErr         error
	scheduleStatus     model.BackupScheduleStatus
	scheduleItems      []model.BackupScheduleStatus
	scheduleErr        error
	activeHealthchecks atomic.Int32
	maxHealthchecks    atomic.Int32
}

func (f *fakeStore) CreateCluster(_ context.Context, request model.CreateClusterRequest) (model.ClusterSummary, error) {
	if f.createErr != nil {
		return model.ClusterSummary{}, f.createErr
	}
	return model.ClusterSummary{
		Namespace: request.Namespace,
		Name:      request.Name,
		Status:    "Provisioning",
	}, nil
}

func (f *fakeStore) ListClusters(context.Context, string) ([]model.ClusterSummary, error) {
	return f.clusters, nil
}

func (f *fakeStore) GetCluster(_ context.Context, namespace, name string) (model.ClusterSummary, error) {
	return model.ClusterSummary{Namespace: namespace, Name: name, Status: "Running"}, nil
}

func (f *fakeStore) GetClusterResources(context.Context, string, string) (model.ClusterResources, error) {
	return f.resources, nil
}

func (f *fakeStore) RunHealthcheck(_ context.Context, namespace, name string) (model.HealthcheckReport, error) {
	if f.healthErr != nil {
		return model.HealthcheckReport{}, f.healthErr
	}
	active := f.activeHealthchecks.Add(1)
	defer f.activeHealthchecks.Add(-1)
	for {
		maximum := f.maxHealthchecks.Load()
		if active <= maximum || f.maxHealthchecks.CompareAndSwap(maximum, active) {
			break
		}
	}
	time.Sleep(f.healthDelay)
	report := model.NewHealthcheckReport(namespace, name)
	report.AddCheck("kubernetes_resources", model.HealthStatusPass, model.HealthSeverityCritical, "resources are ready", nil, time.Now())
	report.Finalize()
	return report, nil
}

func (f *fakeStore) GetMetricsSummary(_ context.Context, namespace, name string) (model.MetricsSummary, error) {
	if f.metricsErr != nil {
		return model.MetricsSummary{}, f.metricsErr
	}
	if f.metricsSummary.Name == "" {
		f.metricsSummary = model.MetricsSummary{Namespace: namespace, Name: name, Cluster: name, Status: "READY"}
	}
	return f.metricsSummary, nil
}

func (f *fakeStore) RunDiagnostics(_ context.Context, namespace, name string, filter model.DiagnosticsFilter) (model.DiagnosticsReport, error) {
	if f.diagnosticsErr != nil {
		return model.DiagnosticsReport{}, f.diagnosticsErr
	}
	if f.diagnosticsReport.Name == "" {
		f.diagnosticsReport = model.NewDiagnosticsReport(namespace, name, filter, model.DefaultDiagnosticsThresholds())
		f.diagnosticsReport.AddFinding("replica", model.DiagnosticSeverityInfo, "Replicas are healthy", nil, "No action required", false)
		f.diagnosticsReport.Finalize()
	}
	return f.diagnosticsReport, nil
}

func (f *fakeStore) CreateBackup(_ context.Context, namespace, name string, _ model.BackupRequest) (model.TaskStatus, error) {
	if f.backupErr != nil {
		return model.TaskStatus{}, f.backupErr
	}
	if f.taskStatus.Name == "" {
		f.taskStatus = model.TaskStatus{Namespace: namespace, Cluster: name, Name: name + "-backup-test", Type: model.TaskTypeBackup, Status: model.TaskStatusPending}
	}
	return f.taskStatus, nil
}

func (f *fakeStore) CreateRestore(_ context.Context, namespace, name string, _ model.RestoreRequest) (model.TaskStatus, error) {
	if f.restoreErr != nil {
		return model.TaskStatus{}, f.restoreErr
	}
	if f.taskStatus.Name == "" {
		f.taskStatus = model.TaskStatus{Namespace: namespace, Cluster: name, Name: name + "-restore-test", Type: model.TaskTypeRestore, Status: model.TaskStatusPending}
	}
	return f.taskStatus, nil
}

func (f *fakeStore) GetTask(_ context.Context, namespace, name, taskName string) (model.TaskStatus, error) {
	if f.taskStatus.Name == "" {
		f.taskStatus = model.TaskStatus{Namespace: namespace, Cluster: name, Name: taskName, Type: model.TaskTypeBackup, Status: model.TaskStatusSucceeded}
	}
	return f.taskStatus, nil
}

func (f *fakeStore) CreateBackupSchedule(_ context.Context, namespace, name string, request model.BackupScheduleRequest) (model.BackupScheduleStatus, error) {
	if f.scheduleErr != nil {
		return model.BackupScheduleStatus{}, f.scheduleErr
	}
	if f.scheduleStatus.Name == "" {
		f.scheduleStatus = model.BackupScheduleStatus{
			Namespace:            namespace,
			Cluster:              name,
			Name:                 request.Name,
			Schedule:             request.Schedule,
			StorageSecretRef:     request.Storage.SecretRef,
			BackupPathPrefix:     request.Storage.PathPrefix,
			SuccessfulJobsRetain: *request.Execution.SuccessfulJobsHistoryLimit,
			FailedJobsRetain:     *request.Execution.FailedJobsHistoryLimit,
		}
	}
	return f.scheduleStatus, nil
}

func (f *fakeStore) ListBackupSchedules(context.Context, string, string) ([]model.BackupScheduleStatus, error) {
	if f.scheduleErr != nil {
		return nil, f.scheduleErr
	}
	return f.scheduleItems, nil
}

func (f *fakeStore) GetBackupSchedule(_ context.Context, namespace, name, scheduleName string) (model.BackupScheduleStatus, error) {
	if f.scheduleErr != nil {
		return model.BackupScheduleStatus{}, f.scheduleErr
	}
	if f.scheduleStatus.Name == "" {
		f.scheduleStatus = model.BackupScheduleStatus{Namespace: namespace, Cluster: name, Name: scheduleName, Schedule: "*/1 * * * *"}
	}
	return f.scheduleStatus, nil
}

func (f *fakeStore) DeleteBackupSchedule(context.Context, string, string, string) error {
	return f.scheduleErr
}

func TestHealthz(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"service":"upm-api-server"`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
	if response.Header().Get("X-Request-Id") == "" {
		t.Fatalf("expected request ID header")
	}
}

func TestCreateClusterRejectsInvalidRequest(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", strings.NewReader(`{"namespace":"bad namespace"}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
	var apiError model.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &apiError); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if apiError.Code != "VALIDATION_ERROR" || apiError.RequestID == "" {
		t.Fatalf("unexpected error response: %#v", apiError)
	}
}

func TestCreateClusterRejectsTrailingJSON(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", strings.NewReader(`{} {}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
	var apiError model.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &apiError); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if apiError.Code != "INVALID_JSON" {
		t.Fatalf("unexpected error response: %#v", apiError)
	}
}

func TestCreateClusterDoesNotEchoSecretMaterial(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	body := []byte(`{
		"namespace":"upm-clickhouse",
		"name":"ch-demo",
		"version":"26.3.9.8",
		"topology":{"shards":1,"replicasPerShard":2,"keeperReplicas":3},
		"storage":{"className":"local-path","serverDataSize":"20Gi","keeperDataSize":"10Gi"},
		"security":{"adminSecretRef":"ch-demo-secret"},
		"monitoring":{"enabled":true}
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", bytes.NewReader(body))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "ch-demo-secret") {
		t.Fatalf("response must not echo secret reference details: %s", response.Body)
	}
}

func TestCreateClusterDoesNotLogRequestSecretMaterial(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	handler := NewServer(&fakeStore{
		createErr: &model.APIError{
			Status:  http.StatusInternalServerError,
			Code:    "BACKEND_ERROR",
			Message: "backend error",
			Err:     errors.New("backend failed"),
		},
	}, logger, time.Second)
	body := []byte(`{
		"namespace":"upm-clickhouse",
		"name":"ch-demo",
		"version":"26.3.9.8",
		"topology":{"shards":1,"replicasPerShard":2,"keeperReplicas":3},
		"storage":{"className":"local-path","serverDataSize":"20Gi","keeperDataSize":"10Gi"},
		"security":{"adminSecretRef":"phase03-secret-should-not-appear-in-logs"},
		"monitoring":{"enabled":true}
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", bytes.NewReader(body))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", response.Code, response.Body)
	}
	if strings.Contains(logBuffer.String(), "phase03-secret-should-not-appear-in-logs") {
		t.Fatalf("log output must not include request secret references: %s", logBuffer.String())
	}
}

func TestListClustersReturnsEmptyArray(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("expected empty array: %s", response.Body)
	}
}

func TestRunHealthcheckStoresLatestReport(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	runRequest := httptest.NewRequest(http.MethodPost, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/healthcheck", nil)
	runResponse := httptest.NewRecorder()

	handler.ServeHTTP(runResponse, runRequest)

	if runResponse.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", runResponse.Code, runResponse.Body)
	}

	latestRequest := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/healthcheck/latest", nil)
	latestResponse := httptest.NewRecorder()
	handler.ServeHTTP(latestResponse, latestRequest)

	if latestResponse.Code != http.StatusOK {
		t.Fatalf("expected latest 200, got %d: %s", latestResponse.Code, latestResponse.Body)
	}
	if !strings.Contains(latestResponse.Body.String(), `"status":"PASS"`) {
		t.Fatalf("unexpected latest report: %s", latestResponse.Body)
	}
}

func TestGetLatestHealthcheckReturnsNotFoundBeforeRun(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/healthcheck/latest", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"code":"HEALTHCHECK_REPORT_NOT_FOUND"`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestRunHealthcheckSerializesSameCluster(t *testing.T) {
	store := &fakeStore{healthDelay: 25 * time.Millisecond}
	handler := newTestServer(store)
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/healthcheck", nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Errorf("expected 200, got %d: %s", response.Code, response.Body)
			}
		}()
	}
	wait.Wait()
	if got := store.maxHealthchecks.Load(); got != 1 {
		t.Fatalf("expected one concurrent healthcheck for the same cluster, got %d", got)
	}
}

func TestGetMetricsSummary(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/metrics/summary", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"status":"READY"`) || !strings.Contains(response.Body.String(), `"requestId":"req-`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestGetMetricsSummaryReturnsPrometheusUnavailable(t *testing.T) {
	handler := newTestServer(&fakeStore{metricsErr: &model.APIError{
		Status:  http.StatusServiceUnavailable,
		Code:    "PROMETHEUS_UNAVAILABLE",
		Message: "Prometheus is unavailable",
	}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/metrics/summary", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"PROMETHEUS_UNAVAILABLE"`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestRunDiagnostics(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/diagnostics?database=upm_healthcheck&table=dist_events&expectedRows=8&limit=10", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"category":"replica"`) || !strings.Contains(response.Body.String(), `"requestId":"req-`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestRunDiagnosticsRejectsInvalidFilter(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/diagnostics?severity=BAD&limit=101", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestCreateBackup(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	body := `{
		"scope":{"database":"upm_backup_validation","table":"events"},
		"storage":{"type":"s3","secretRef":"clickhouse-backup-secret","path":"backups/ch/manual"},
		"execution":{"type":"kubernetesJob"}
	}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/backup", strings.NewReader(body))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"type":"backup"`) || !strings.Contains(response.Body.String(), `"requestId":"req-`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestCreateRestoreRequiresConfirm(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	body := `{
		"backupRef":"backups/ch/manual",
		"source":{"database":"upm_backup_validation","table":"events"},
		"target":{"database":"restore_validation","table":"events_restored"},
		"storage":{"type":"s3","secretRef":"clickhouse-backup-secret"},
		"execution":{"type":"kubernetesJob"},
		"reason":"restore validation drill"
	}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/restore", strings.NewReader(body))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"confirm must be true`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestCreateBackupSchedule(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	body := `{
		"name":"validation-every-minute",
		"schedule":"*/1 * * * *",
		"scope":{"database":"upm_backup_validation","table":"events"},
		"storage":{"type":"s3","secretRef":"clickhouse-backup-secret","pathPrefix":"backups/ch/scheduled"},
		"execution":{"type":"kubernetesCronJob","concurrencyPolicy":"Forbid"}
	}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/backup-schedules", strings.NewReader(body))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"name":"validation-every-minute"`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestGetTask(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/tasks/clickhouse-phase03-backup-test", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"Succeeded"`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestListBackupSchedulesReturnsEmptyArray(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/backup-schedules", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestDeleteBackupSchedule(t *testing.T) {
	handler := newTestServer(&fakeStore{})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/upm-clickhouse/clickhouse-phase03/backup-schedules/validation-every-minute", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", response.Code, response.Body)
	}
}

func newTestServer(store *fakeStore) http.Handler {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return NewServer(store, logger, time.Second)
}
