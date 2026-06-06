package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/model"
)

type fakeStore struct {
	clusters  []model.ClusterSummary
	resources model.ClusterResources
	createErr error
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

func newTestServer(store *fakeStore) http.Handler {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return NewServer(store, logger, time.Second)
}
