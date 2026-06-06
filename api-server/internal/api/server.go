package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/model"
	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/platform"
)

type contextKey string

const requestIDKey contextKey = "requestID"

type Server struct {
	store   platform.Store
	logger  *slog.Logger
	timeout time.Duration

	latestMu sync.RWMutex
	latest   map[string]model.HealthcheckReport

	healthcheckLocksMu sync.Mutex
	healthcheckLocks   map[string]*sync.Mutex
}

func NewServer(store platform.Store, logger *slog.Logger, timeout time.Duration) http.Handler {
	server := &Server{
		store:            store,
		logger:           logger,
		timeout:          timeout,
		latest:           map[string]model.HealthcheckReport{},
		healthcheckLocks: map[string]*sync.Mutex{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/healthz", server.healthz)
	mux.HandleFunc("POST /api/v1/clusters", server.createCluster)
	mux.HandleFunc("GET /api/v1/clusters", server.listClusters)
	mux.HandleFunc("GET /api/v1/clusters/{namespace}/{name}", server.getCluster)
	mux.HandleFunc("GET /api/v1/clusters/{namespace}/{name}/resources", server.getClusterResources)
	mux.HandleFunc("POST /api/v1/clusters/{namespace}/{name}/healthcheck", server.runHealthcheck)
	mux.HandleFunc("GET /api/v1/clusters/{namespace}/{name}/healthcheck/latest", server.getLatestHealthcheck)
	mux.HandleFunc("GET /api/v1/clusters/{namespace}/{name}/metrics/summary", server.getMetricsSummary)
	return server.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-Id", requestID)
		w.Header().Set("Content-Type", "application/json")

		ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
		defer cancel()
		ctx = context.WithValue(ctx, requestIDKey, requestID)

		started := time.Now()
		next.ServeHTTP(w, r.WithContext(ctx))
		s.logger.Info("request completed",
			"requestId", requestID,
			"actor", r.Header.Get("X-Actor"),
			"method", r.Method,
			"path", r.URL.Path,
			"durationMs", time.Since(started).Milliseconds(),
		)
	})
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "upm-api-server",
	})
}

func (s *Server) createCluster(w http.ResponseWriter, r *http.Request) {
	var request model.CreateClusterRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		s.writeError(w, r, &model.APIError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_JSON",
			Message: "request body must be valid JSON",
			Err:     err,
		})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		s.writeError(w, r, &model.APIError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_JSON",
			Message: "request body must contain one JSON object",
		})
		return
	}
	if err := request.Validate(); err != nil {
		s.writeError(w, r, &model.APIError{
			Status:  http.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: err.Error(),
			Err:     err,
		})
		return
	}

	cluster, err := s.store.CreateCluster(r.Context(), request)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	cluster.RequestID = requestID(r.Context())
	writeJSON(w, http.StatusAccepted, cluster)
}

func (s *Server) listClusters(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	if namespace != "" {
		if err := model.ValidateNamespace(namespace); err != nil {
			s.writeValidationError(w, r, err)
			return
		}
	}
	clusters, err := s.store.ListClusters(r.Context(), namespace)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if clusters == nil {
		clusters = []model.ClusterSummary{}
	}
	writeJSON(w, http.StatusOK, model.ClusterList{Items: clusters})
}

func (s *Server) getCluster(w http.ResponseWriter, r *http.Request) {
	namespace, name := r.PathValue("namespace"), r.PathValue("name")
	if err := model.ValidateClusterIdentity(namespace, name); err != nil {
		s.writeValidationError(w, r, err)
		return
	}
	cluster, err := s.store.GetCluster(r.Context(), namespace, name)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cluster)
}

func (s *Server) getClusterResources(w http.ResponseWriter, r *http.Request) {
	namespace, name := r.PathValue("namespace"), r.PathValue("name")
	if err := model.ValidateClusterIdentity(namespace, name); err != nil {
		s.writeValidationError(w, r, err)
		return
	}
	resources, err := s.store.GetClusterResources(r.Context(), namespace, name)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resources)
}

func (s *Server) runHealthcheck(w http.ResponseWriter, r *http.Request) {
	namespace, name := r.PathValue("namespace"), r.PathValue("name")
	if err := model.ValidateClusterIdentity(namespace, name); err != nil {
		s.writeValidationError(w, r, err)
		return
	}
	lock := s.healthcheckLock(healthcheckKey(namespace, name))
	lock.Lock()
	defer lock.Unlock()

	report, err := s.store.RunHealthcheck(r.Context(), namespace, name)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	report.RequestID = requestID(r.Context())

	s.latestMu.Lock()
	s.latest[healthcheckKey(namespace, name)] = report
	s.latestMu.Unlock()

	writeJSON(w, http.StatusOK, report)
}

func (s *Server) getLatestHealthcheck(w http.ResponseWriter, r *http.Request) {
	namespace, name := r.PathValue("namespace"), r.PathValue("name")
	if err := model.ValidateClusterIdentity(namespace, name); err != nil {
		s.writeValidationError(w, r, err)
		return
	}

	s.latestMu.RLock()
	report, exists := s.latest[healthcheckKey(namespace, name)]
	s.latestMu.RUnlock()
	if !exists {
		s.writeError(w, r, &model.APIError{
			Status:  http.StatusNotFound,
			Code:    "HEALTHCHECK_REPORT_NOT_FOUND",
			Message: "latest healthcheck report not found",
			Details: map[string]any{"namespace": namespace, "name": name},
		})
		return
	}
	report.RequestID = requestID(r.Context())
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) getMetricsSummary(w http.ResponseWriter, r *http.Request) {
	namespace, name := r.PathValue("namespace"), r.PathValue("name")
	if err := model.ValidateClusterIdentity(namespace, name); err != nil {
		s.writeValidationError(w, r, err)
		return
	}
	summary, err := s.store.GetMetricsSummary(r.Context(), namespace, name)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	summary.RequestID = requestID(r.Context())
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) writeValidationError(w http.ResponseWriter, r *http.Request, err error) {
	s.writeError(w, r, &model.APIError{
		Status:  http.StatusBadRequest,
		Code:    "VALIDATION_ERROR",
		Message: err.Error(),
		Err:     err,
	})
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	apiError := &model.APIError{
		Status:  http.StatusInternalServerError,
		Code:    "INTERNAL_ERROR",
		Message: "internal server error",
		Err:     err,
	}
	if errors.As(err, &apiError) {
		if apiError.Status == 0 {
			apiError.Status = http.StatusInternalServerError
		}
	}
	s.logger.Error("request failed",
		"requestId", requestID(r.Context()),
		"code", apiError.Code,
		"error", apiError.Error(),
	)
	writeJSON(w, apiError.Status, model.ErrorResponse{
		Code:      apiError.Code,
		Message:   apiError.Message,
		Details:   apiError.Details,
		RequestID: requestID(r.Context()),
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func newRequestID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "req-unknown"
	}
	return "req-" + hex.EncodeToString(data[:])
}

func healthcheckKey(namespace, name string) string {
	return namespace + "/" + name
}

func (s *Server) healthcheckLock(key string) *sync.Mutex {
	s.healthcheckLocksMu.Lock()
	defer s.healthcheckLocksMu.Unlock()
	lock, exists := s.healthcheckLocks[key]
	if !exists {
		lock = &sync.Mutex{}
		s.healthcheckLocks[key] = lock
	}
	return lock
}
