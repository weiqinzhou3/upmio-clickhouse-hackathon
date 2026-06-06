package kube

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/yaml"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/model"
	promclient "github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/prometheus"
)

const (
	serviceGroupNameLabel = "upm.api/service-group.name"
	serviceGroupTypeLabel = "upm.api/service-group.type"
	serviceTypeLabel      = "upm.api/service.type"

	topologyShardsAnnotation           = "upm.api/clickhouse.topology.shards"
	topologyReplicasPerShardAnnotation = "upm.api/clickhouse.topology.replicas-per-shard"
	topologyKeeperReplicasAnnotation   = "upm.api/clickhouse.topology.keeper-replicas"
	adminSecretAnnotation              = "upm.api/clickhouse.admin-secret"
	storageClassAnnotation             = "upm.api/clickhouse.storage-class"
	serverDataSizeAnnotation           = "upm.api/clickhouse.server-data-size"
	keeperDataSizeAnnotation           = "upm.api/clickhouse.keeper-data-size"
	monitoringEnabledAnnotation        = "upm.api/clickhouse.monitoring-enabled"
	keeperServiceNameAnnotation        = "upm.api/clickhouse-keeper.service-name"
	managerNamespace                   = "upm-system"
	clickHouseClusterName              = "upm_cluster"
	healthcheckDatabase                = "upm_healthcheck"
	healthcheckLocalTable              = "local_events"
	healthcheckDistributedTable        = "dist_events"
)

var (
	projectGVR = schema.GroupVersionResource{
		Group: "upm.syntropycloud.io", Version: "v1alpha2", Resource: "projects",
	}
	unitSetGVR = schema.GroupVersionResource{
		Group: "upm.syntropycloud.io", Version: "v1alpha2", Resource: "unitsets",
	}
	unitGVR = schema.GroupVersionResource{
		Group: "upm.syntropycloud.io", Version: "v1alpha2", Resource: "units",
	}
	podMonitorGVR = schema.GroupVersionResource{
		Group: "monitoring.coreos.com", Version: "v1", Resource: "podmonitors",
	}
)

type prometheusQuerier interface {
	Query(context.Context, string) ([]promclient.Sample, error)
}

type Store struct {
	dynamic    dynamic.Interface
	core       kubernetes.Interface
	restConfig *rest.Config
	prometheus prometheusQuerier
}

func NewInClusterStore(prometheusClient prometheusQuerier) (*Store, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("load in-cluster Kubernetes config: %w", err)
	}
	config.UserAgent = "upm-api-server"

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic Kubernetes client: %w", err)
	}
	coreClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create core Kubernetes client: %w", err)
	}
	return &Store{dynamic: dynamicClient, core: coreClient, restConfig: config, prometheus: prometheusClient}, nil
}

func NewStore(dynamicClient dynamic.Interface, coreClient kubernetes.Interface) *Store {
	return &Store{dynamic: dynamicClient, core: coreClient}
}

func NewStoreWithPrometheus(dynamicClient dynamic.Interface, coreClient kubernetes.Interface, prometheusClient prometheusQuerier) *Store {
	return &Store{dynamic: dynamicClient, core: coreClient, prometheus: prometheusClient}
}

func (s *Store) CreateCluster(ctx context.Context, request model.CreateClusterRequest) (model.ClusterSummary, error) {
	selector := labels.Set{serviceGroupNameLabel: request.Name}.AsSelector().String()
	existing, err := s.dynamic.Resource(unitSetGVR).Namespace(request.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return model.ClusterSummary{}, internalError("list existing UnitSets", err)
	}
	keeperExists := false
	if err == nil {
		for i := range existing.Items {
			switch existing.Items[i].GetName() {
			case request.Name:
				return model.ClusterSummary{}, &model.APIError{
					Status:  http.StatusConflict,
					Code:    "CLUSTER_ALREADY_EXISTS",
					Message: "a managed cluster with this namespace and name already exists",
				}
			case request.Name + "-keeper":
				version, _, _ := unstructured.NestedString(existing.Items[i].Object, "spec", "version")
				units := nestedInt64Or(existing.Items[i].Object, 0, "spec", "units")
				if version != request.Version || units != int64(request.Topology.KeeperReplicas) {
					return model.ClusterSummary{}, &model.APIError{
						Status:  http.StatusConflict,
						Code:    "CLUSTER_RESOURCE_CONFLICT",
						Message: "existing Keeper UnitSet does not match the requested version or topology",
					}
				}
				keeperExists = true
			default:
				return model.ClusterSummary{}, &model.APIError{
					Status:  http.StatusConflict,
					Code:    "CLUSTER_RESOURCE_CONFLICT",
					Message: "another managed resource already uses this cluster service-group name",
				}
			}
		}
	}

	if err := s.validatePackagePrerequisites(ctx, request); err != nil {
		return model.ClusterSummary{}, err
	}

	if err := s.ensureNamespace(ctx, request.Namespace); err != nil {
		return model.ClusterSummary{}, err
	}
	if err := s.validateSecretKey(ctx, request.Namespace, request.Security.AdminSecretRef, "CLICKHOUSE_ADMIN_PASSWORD", "ADMIN_SECRET"); err != nil {
		return model.ClusterSummary{}, err
	}
	if err := s.validateSecretKey(ctx, request.Namespace, "aes-secret-key", "AES_SECRET_KEY", "AES_SECRET"); err != nil {
		return model.ClusterSummary{}, err
	}

	if err := s.ensureProject(ctx, request.Namespace); err != nil {
		return model.ClusterSummary{}, err
	}

	if !keeperExists {
		keeper := keeperUnitSet(request)
		if _, err := s.dynamic.Resource(unitSetGVR).Namespace(request.Namespace).Create(ctx, keeper, metav1.CreateOptions{}); err != nil {
			return model.ClusterSummary{}, internalError("create Keeper UnitSet", err)
		}
	}
	if err := s.waitUnitSetReady(ctx, request.Namespace, request.Name+"-keeper", request.Topology.KeeperReplicas); err != nil {
		return model.ClusterSummary{}, err
	}

	server := serverUnitSet(request)
	if _, err := s.dynamic.Resource(unitSetGVR).Namespace(request.Namespace).Create(ctx, server, metav1.CreateOptions{}); err != nil {
		return model.ClusterSummary{}, internalError("create ClickHouse Server UnitSet", err)
	}

	return model.ClusterSummary{
		Namespace: request.Namespace,
		Name:      request.Name,
		Version:   request.Version,
		Status:    "Provisioning",
		Topology:  request.Topology,
		Ready: model.ReadySummary{
			Keeper: fmt.Sprintf("%d/%d", request.Topology.KeeperReplicas, request.Topology.KeeperReplicas),
			Server: fmt.Sprintf("0/%d", request.Topology.Shards*request.Topology.ReplicasPerShard),
		},
		Resources: model.ClusterResourceRef{
			Project:       request.Namespace,
			KeeperUnitSet: request.Name + "-keeper",
			ServerUnitSet: request.Name,
		},
	}, nil
}

func (s *Store) validateSecretKey(ctx context.Context, namespace, name, key, codePrefix string) error {
	secret, err := s.core.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &model.APIError{
				Status:  http.StatusUnprocessableEntity,
				Code:    codePrefix + "_NOT_FOUND",
				Message: fmt.Sprintf("required Secret %s/%s does not exist", namespace, name),
			}
		}
		return internalError("check Secret reference", err)
	}
	if _, exists := secret.Data[key]; !exists {
		return &model.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    codePrefix + "_KEY_MISSING",
			Message: fmt.Sprintf("required key %s is missing from Secret %s/%s", key, namespace, name),
		}
	}
	return nil
}

func (s *Store) waitUnitSetReady(ctx context.Context, namespace, name string, expected int) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		unitSet, err := s.dynamic.Resource(unitSetGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return internalError("wait for UnitSet readiness", err)
		}
		if nestedInt64Or(unitSet.Object, 0, "status", "readyUnits") == int64(expected) {
			return nil
		}
		select {
		case <-ctx.Done():
			return &model.APIError{
				Status:  http.StatusGatewayTimeout,
				Code:    "UPMIO_UNITSET_NOT_READY",
				Message: fmt.Sprintf("UnitSet %s/%s did not become ready before the request timeout", namespace, name),
				Err:     ctx.Err(),
			}
		case <-ticker.C:
		}
	}
}

func (s *Store) ListClusters(ctx context.Context, namespace string) ([]model.ClusterSummary, error) {
	resource := s.dynamic.Resource(unitSetGVR)
	var list *unstructured.UnstructuredList
	var err error
	options := metav1.ListOptions{
		LabelSelector: labels.Set{
			serviceGroupTypeLabel: "clickhouse-sg",
			serviceTypeLabel:      "clickhouse",
		}.AsSelector().String(),
	}
	if namespace == "" {
		list, err = resource.Namespace(metav1.NamespaceAll).List(ctx, options)
	} else {
		list, err = resource.Namespace(namespace).List(ctx, options)
	}
	if err != nil {
		return nil, internalError("list ClickHouse UnitSets", err)
	}

	clusters := make([]model.ClusterSummary, 0, len(list.Items))
	for i := range list.Items {
		cluster, err := s.summaryFromServerUnitSet(ctx, &list.Items[i])
		if err != nil {
			return nil, err
		}
		clusters = append(clusters, cluster)
	}
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].Namespace == clusters[j].Namespace {
			return clusters[i].Name < clusters[j].Name
		}
		return clusters[i].Namespace < clusters[j].Namespace
	})
	return clusters, nil
}

func (s *Store) GetCluster(ctx context.Context, namespace, name string) (model.ClusterSummary, error) {
	server, err := s.dynamic.Resource(unitSetGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return model.ClusterSummary{}, notFoundError(namespace, name)
		}
		return model.ClusterSummary{}, internalError("get ClickHouse UnitSet", err)
	}
	if server.GetLabels()[serviceGroupNameLabel] != name || server.GetLabels()[serviceTypeLabel] != "clickhouse" {
		return model.ClusterSummary{}, notFoundError(namespace, name)
	}
	return s.summaryFromServerUnitSet(ctx, server)
}

func (s *Store) GetClusterResources(ctx context.Context, namespace, name string) (model.ClusterResources, error) {
	if _, err := s.GetCluster(ctx, namespace, name); err != nil {
		return model.ClusterResources{}, err
	}
	result := model.ClusterResources{
		Namespace: namespace,
		Name:      name,
		UnitSets:  []model.ResourceSummary{},
		Units:     []model.ResourceSummary{},
		Pods:      []model.ResourceSummary{},
		PVCs:      []model.ResourceSummary{},
		Services:  []model.ResourceSummary{},
		Endpoints: []model.ResourceSummary{},
		Events:    []model.ResourceSummary{},
	}

	selector := labels.Set{serviceGroupNameLabel: name}.AsSelector().String()
	unitSets, err := s.dynamic.Resource(unitSetGVR).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return model.ClusterResources{}, internalError("list UnitSets", err)
	}
	for i := range unitSets.Items {
		result.UnitSets = append(result.UnitSets, unstructuredSummary(&unitSets.Items[i], "UnitSet"))
	}

	units, err := s.dynamic.Resource(unitGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return model.ClusterResources{}, internalError("list Units", err)
	}
	for i := range units.Items {
		if resourceMatches(units.Items[i].GetName(), units.Items[i].GetLabels(), name) {
			result.Units = append(result.Units, unstructuredSummary(&units.Items[i], "Unit"))
		}
	}

	pods, err := s.core.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return model.ClusterResources{}, internalError("list Pods", err)
	}
	for i := range pods.Items {
		if resourceMatches(pods.Items[i].Name, pods.Items[i].Labels, name) {
			result.Pods = append(result.Pods, podSummary(&pods.Items[i]))
		}
	}

	pvcs, err := s.core.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return model.ClusterResources{}, internalError("list PVCs", err)
	}
	for i := range pvcs.Items {
		if resourceMatches(pvcs.Items[i].Name, pvcs.Items[i].Labels, name) {
			result.PVCs = append(result.PVCs, pvcSummary(&pvcs.Items[i]))
		}
	}

	services, err := s.core.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return model.ClusterResources{}, internalError("list Services", err)
	}
	for i := range services.Items {
		if resourceMatches(services.Items[i].Name, services.Items[i].Labels, name) {
			result.Services = append(result.Services, serviceSummary(&services.Items[i]))
		}
	}

	endpoints, err := s.core.CoreV1().Endpoints(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return model.ClusterResources{}, internalError("list Endpoints", err)
	}
	for i := range endpoints.Items {
		if resourceMatches(endpoints.Items[i].Name, endpoints.Items[i].Labels, name) {
			result.Endpoints = append(result.Endpoints, endpointSummary(&endpoints.Items[i]))
		}
	}

	events, err := s.core.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return model.ClusterResources{}, internalError("list Events", err)
	}
	for i := range events.Items {
		if resourceNameMatches(events.Items[i].InvolvedObject.Name, name) {
			result.Events = append(result.Events, eventSummary(&events.Items[i]))
		}
	}
	return result, nil
}

func (s *Store) RunHealthcheck(ctx context.Context, namespace, name string) (model.HealthcheckReport, error) {
	cluster, err := s.GetCluster(ctx, namespace, name)
	if err != nil {
		return model.HealthcheckReport{}, err
	}

	report := model.NewHealthcheckReport(namespace, name)
	sensitiveValues, sensitiveErr := s.collectHealthcheckSensitiveValues(ctx, namespace, name)
	s.checkProject(ctx, &report, namespace)
	s.checkUnitSetReady(ctx, &report, namespace, name+"-keeper", cluster.Topology.KeeperReplicas, "keeper_unitset_ready")
	s.checkUnitSetReady(ctx, &report, namespace, name, cluster.Topology.Shards*cluster.Topology.ReplicasPerShard, "server_unitset_ready")

	keeperPods, serverPods, err := s.listClusterPods(ctx, namespace, name)
	if err != nil {
		addFailure(&report, "pods_ready", model.HealthSeverityCritical, "cannot list cluster pods", err, time.Now())
	} else {
		s.checkPodsReady(&report, cluster.Topology, keeperPods, serverPods)
	}
	s.checkPVCsBound(ctx, &report, namespace, name, cluster.Topology)
	s.checkServicesEndpoints(ctx, &report, namespace, name)
	s.checkKeeperRUOK(ctx, &report, namespace, keeperPods)
	s.checkKeeperRoles(ctx, &report, namespace, keeperPods)
	s.checkClickHouseSelect(ctx, &report, namespace, serverPods)
	s.checkSystemClusters(ctx, &report, namespace, serverPods, cluster.Topology)
	s.checkWriteReadProbe(ctx, &report, namespace, serverPods, cluster.Topology)
	s.checkReplicaHealth(ctx, &report, namespace, serverPods, cluster.Topology)
	s.checkMetricsEndpoint(ctx, &report, namespace, serverPods)
	redactHealthcheckReport(&report, sensitiveValues)
	addSecretLeakageCheck(&report, sensitiveValues, sensitiveErr)
	report.Finalize()
	return report, nil
}

func (s *Store) GetMetricsSummary(ctx context.Context, namespace, name string) (model.MetricsSummary, error) {
	if _, err := s.GetCluster(ctx, namespace, name); err != nil {
		return model.MetricsSummary{}, err
	}
	if s.prometheus == nil {
		return model.MetricsSummary{}, prometheusUnavailableError(errors.New("Prometheus client is not configured"))
	}

	_, serverPods, err := s.listClusterPods(ctx, namespace, name)
	if err != nil {
		return model.MetricsSummary{}, internalError("list expected ClickHouse Pods", err)
	}
	expectedPods := make([]string, 0, len(serverPods))
	for _, pod := range serverPods {
		expectedPods = append(expectedPods, pod.Name)
	}
	sort.Strings(expectedPods)
	podPattern := regexp.QuoteMeta(name) + `-[0-9]+`

	summary := model.MetricsSummary{
		Namespace:   namespace,
		Name:        name,
		Cluster:     name,
		Status:      "READY",
		CollectedAt: time.Now().UTC(),
		PodMonitor: model.MetricsPodMonitor{
			Name: name + "-exporter-podmon",
		},
		Targets:  make([]model.MetricsTarget, 0, len(expectedPods)),
		Warnings: []string{},
		Summary: model.MetricsCategorySummary{
			CPU:        []model.MetricSample{},
			Memory:     []model.MetricSample{},
			Storage:    []model.MetricSample{},
			ClickHouse: []model.MetricSample{},
		},
	}

	if _, err := s.dynamic.Resource(podMonitorGVR).Namespace(namespace).Get(ctx, summary.PodMonitor.Name, metav1.GetOptions{}); err == nil {
		summary.PodMonitor.Exists = true
	} else {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("PodMonitor %s/%s is not readable or does not exist", namespace, summary.PodMonitor.Name))
	}

	targetQuery := fmt.Sprintf(`up{namespace=%q,pod=~%q}`, namespace, podPattern)
	targetSamples, err := s.prometheus.Query(ctx, targetQuery)
	if err != nil {
		return model.MetricsSummary{}, prometheusUnavailableError(err)
	}
	targetState := make(map[string]bool, len(targetSamples))
	for _, sample := range targetSamples {
		if pod := sample.Metric["pod"]; pod != "" {
			targetState[pod] = targetState[pod] || sample.Value == 1
		}
	}
	for _, pod := range expectedPods {
		up, exists := targetState[pod]
		summary.Targets = append(summary.Targets, model.MetricsTarget{Name: pod, Up: exists && up})
		if !exists {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("Prometheus target for %s is missing", pod))
		} else if !up {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("Prometheus target for %s is down", pod))
		}
	}

	queries := []struct {
		category string
		query    string
		unit     string
		assign   func([]model.MetricSample)
	}{
		{
			category: "cpu",
			query:    fmt.Sprintf(`sum by (pod) (rate(container_cpu_usage_seconds_total{namespace=%q,pod=~%q,container="clickhouse"}[2m]))`, namespace, podPattern),
			unit:     "cores",
			assign:   func(samples []model.MetricSample) { summary.Summary.CPU = samples },
		},
		{
			category: "memory",
			query:    fmt.Sprintf(`container_memory_working_set_bytes{namespace=%q,pod=~%q,container="clickhouse"}`, namespace, podPattern),
			unit:     "bytes",
			assign:   func(samples []model.MetricSample) { summary.Summary.Memory = samples },
		},
		{
			category: "clickhouse",
			query: fmt.Sprintf(`{__name__=~"ClickHouseProfileEvents_(Query|InsertQuery|InsertedRows|InsertedBytes)|ClickHouseMetrics_MemoryTracking",namespace=%q,pod=~%q}`,
				namespace, podPattern),
			assign: func(samples []model.MetricSample) { summary.Summary.ClickHouse = samples },
		},
	}
	for _, item := range queries {
		samples, queryErr := s.prometheus.Query(ctx, item.query)
		if queryErr != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s metrics query failed", item.category))
			continue
		}
		converted := metricSamples(samples, item.unit)
		item.assign(converted)
		if len(converted) == 0 {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s metrics are missing", item.category))
		}
	}

	storageSamples, storageErr := firstAvailableMetrics(ctx, s.prometheus,
		fmt.Sprintf(`kubelet_volume_stats_used_bytes{namespace=%q,persistentvolumeclaim=~%q}`, namespace, regexp.QuoteMeta(name)+`-[0-9]+-data`),
		fmt.Sprintf(`{__name__=~"ClickHouseAsyncMetrics_Disk(Used|Total|Available)_default",namespace=%q,pod=~%q}`, namespace, podPattern),
	)
	if storageErr != nil {
		summary.Warnings = append(summary.Warnings, "storage metrics queries failed")
	} else {
		summary.Summary.Storage = metricSamples(storageSamples, "bytes")
		if len(summary.Summary.Storage) == 0 {
			summary.Warnings = append(summary.Warnings, "storage metrics are missing")
		}
	}

	if !summary.PodMonitor.Exists || len(expectedPods) == 0 || len(summary.Warnings) > 0 {
		summary.Status = "DEGRADED"
	}
	return summary, nil
}

func firstAvailableMetrics(ctx context.Context, querier prometheusQuerier, queries ...string) ([]promclient.Sample, error) {
	var lastErr error
	for _, query := range queries {
		samples, err := querier.Query(ctx, query)
		if err != nil {
			lastErr = err
			continue
		}
		if len(samples) > 0 {
			return samples, nil
		}
	}
	return nil, lastErr
}

func metricSamples(samples []promclient.Sample, unit string) []model.MetricSample {
	allowedLabels := map[string]bool{
		"namespace":             true,
		"pod":                   true,
		"node":                  true,
		"persistentvolumeclaim": true,
	}
	result := make([]model.MetricSample, 0, len(samples))
	for _, sample := range samples {
		name := sample.Metric["pod"]
		if name == "" {
			name = sample.Metric["persistentvolumeclaim"]
		}
		if metricName := sample.Metric["__name__"]; metricName != "" {
			name = metricName
		}
		metricLabels := make(map[string]string, len(sample.Metric))
		for key, value := range sample.Metric {
			if allowedLabels[key] {
				metricLabels[key] = value
			}
		}
		result = append(result, model.MetricSample{
			Name:   name,
			Labels: metricLabels,
			Value:  sample.Value,
			Unit:   unit,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].Labels["pod"] < result[j].Labels["pod"]
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func prometheusUnavailableError(err error) error {
	return &model.APIError{
		Status:  http.StatusServiceUnavailable,
		Code:    "PROMETHEUS_UNAVAILABLE",
		Message: "Prometheus is unavailable",
		Err:     err,
	}
}

func (s *Store) checkProject(ctx context.Context, report *model.HealthcheckReport, namespace string) {
	started := time.Now()
	_, err := s.dynamic.Resource(projectGVR).Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			addFailure(report, "upmio_project_exists", model.HealthSeverityCritical, "UPMIO Project does not exist", err, started)
			return
		}
		addFailure(report, "upmio_project_exists", model.HealthSeverityCritical, "cannot read UPMIO Project", err, started)
		return
	}
	report.AddCheck("upmio_project_exists", model.HealthStatusPass, model.HealthSeverityCritical, "UPMIO Project exists", map[string]any{
		"project": namespace,
	}, started)
}

func (s *Store) checkUnitSetReady(ctx context.Context, report *model.HealthcheckReport, namespace, name string, expected int, checkName string) {
	started := time.Now()
	unitSet, err := s.dynamic.Resource(unitSetGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		addFailure(report, checkName, model.HealthSeverityCritical, "UnitSet cannot be read", err, started)
		return
	}
	ready := int(nestedInt64Or(unitSet.Object, 0, "status", "readyUnits"))
	current := int(nestedInt64Or(unitSet.Object, nestedInt64Or(unitSet.Object, 0, "status", "units"), "status", "currentUnits"))
	specUnits := int(nestedInt64Or(unitSet.Object, int64(expected), "spec", "units"))
	status := model.HealthStatusPass
	message := "UnitSet has the expected ready units"
	if specUnits != expected || current != expected || ready != expected {
		status = model.HealthStatusFail
		message = "UnitSet ready units do not match expected topology"
	}
	report.AddCheck(checkName, status, model.HealthSeverityCritical, message, map[string]any{
		"namespace": namespace,
		"unitSet":   name,
		"expected":  expected,
		"specUnits": specUnits,
		"current":   current,
		"ready":     ready,
	}, started)
}

func (s *Store) listClusterPods(ctx context.Context, namespace, name string) ([]corev1.Pod, []corev1.Pod, error) {
	pods, err := s.core.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, internalError("list Pods", err)
	}
	keeperPods := []corev1.Pod{}
	serverPods := []corev1.Pod{}
	for i := range pods.Items {
		pod := pods.Items[i]
		if !resourceMatches(pod.Name, pod.Labels, name) {
			continue
		}
		if strings.HasPrefix(pod.Name, name+"-keeper-") {
			keeperPods = append(keeperPods, pod)
			continue
		}
		serverPods = append(serverPods, pod)
	}
	sort.Slice(keeperPods, func(i, j int) bool { return keeperPods[i].Name < keeperPods[j].Name })
	sort.Slice(serverPods, func(i, j int) bool { return serverPods[i].Name < serverPods[j].Name })
	return keeperPods, serverPods, nil
}

func (s *Store) checkPodsReady(report *model.HealthcheckReport, topology model.Topology, keeperPods, serverPods []corev1.Pod) {
	started := time.Now()
	serverExpected := topology.Shards * topology.ReplicasPerShard
	keeperExpected := topology.KeeperReplicas
	notReady := []string{}
	for _, pod := range append(append([]corev1.Pod{}, keeperPods...), serverPods...) {
		if !podIsReady(&pod) {
			notReady = append(notReady, pod.Name)
		}
	}
	status := model.HealthStatusPass
	message := "all ClickHouse and Keeper pods are ready"
	if len(keeperPods) != keeperExpected || len(serverPods) != serverExpected || len(notReady) > 0 {
		status = model.HealthStatusFail
		message = "pod count or readiness does not match expected topology"
	}
	report.AddCheck("pods_ready", status, model.HealthSeverityCritical, message, map[string]any{
		"expectedKeeperPods": keeperExpected,
		"actualKeeperPods":   len(keeperPods),
		"expectedServerPods": serverExpected,
		"actualServerPods":   len(serverPods),
		"notReadyPods":       notReady,
		"keeperPods":         podNames(keeperPods),
		"serverPods":         podNames(serverPods),
	}, started)
}

func (s *Store) checkPVCsBound(ctx context.Context, report *model.HealthcheckReport, namespace, name string, topology model.Topology) {
	started := time.Now()
	pvcs, err := s.core.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		addFailure(report, "pvc_bound", model.HealthSeverityCritical, "cannot list PVCs", err, started)
		return
	}
	expected := topology.KeeperReplicas + topology.Shards*topology.ReplicasPerShard
	matched := []string{}
	notBound := []string{}
	for i := range pvcs.Items {
		pvc := pvcs.Items[i]
		if !storageResourceNameMatches(pvc.Name, name) {
			continue
		}
		matched = append(matched, pvc.Name)
		if pvc.Status.Phase != corev1.ClaimBound {
			notBound = append(notBound, pvc.Name)
		}
	}
	status := model.HealthStatusPass
	message := "all data PVCs are bound"
	if len(matched) != expected || len(notBound) > 0 {
		status = model.HealthStatusFail
		message = "PVC count or binding status does not match expected topology"
	}
	report.AddCheck("pvc_bound", status, model.HealthSeverityCritical, message, map[string]any{
		"expectedPVCs": expected,
		"actualPVCs":   len(matched),
		"notBoundPVCs": notBound,
		"pvcs":         matched,
	}, started)
}

func (s *Store) checkServicesEndpoints(ctx context.Context, report *model.HealthcheckReport, namespace, name string) {
	started := time.Now()
	requiredServerPorts := map[string]int32{
		"tcp":         9000,
		"http":        8123,
		"interserver": 9009,
		"metrics":     9363,
	}
	services, err := s.core.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		addFailure(report, "services_endpoints", model.HealthSeverityCritical, "cannot list Services", err, started)
		return
	}
	endpoints, err := s.core.CoreV1().Endpoints(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		addFailure(report, "services_endpoints", model.HealthSeverityCritical, "cannot list Endpoints", err, started)
		return
	}
	endpointReady := map[string]int{}
	endpointPorts := map[string]map[string]int32{}
	for i := range endpoints.Items {
		endpoint := endpoints.Items[i]
		if !resourceMatches(endpoint.Name, endpoint.Labels, name) {
			continue
		}
		ready := 0
		for _, subset := range endpoint.Subsets {
			ready += len(subset.Addresses)
			if endpointPorts[endpoint.Name] == nil {
				endpointPorts[endpoint.Name] = map[string]int32{}
			}
			for _, port := range subset.Ports {
				endpointPorts[endpoint.Name][port.Name] = port.Port
			}
		}
		endpointReady[endpoint.Name] = ready
	}
	serviceNames := []string{}
	missingReadyEndpoints := []string{}
	missingServicePorts := map[string][]string{}
	missingEndpointPorts := map[string][]string{}
	for i := range services.Items {
		service := services.Items[i]
		if !resourceMatches(service.Name, service.Labels, name) {
			continue
		}
		serviceNames = append(serviceNames, service.Name)
		if endpointReady[service.Name] == 0 {
			missingReadyEndpoints = append(missingReadyEndpoints, service.Name)
		}
		if strings.HasPrefix(service.Name, name+"-keeper-") {
			continue
		}
		servicePorts := map[string]int32{}
		for _, port := range service.Spec.Ports {
			servicePorts[port.Name] = port.Port
		}
		for portName, portNumber := range requiredServerPorts {
			if servicePorts[portName] != portNumber {
				missingServicePorts[service.Name] = append(missingServicePorts[service.Name], portName)
			}
			if endpointPorts[service.Name][portName] != portNumber {
				missingEndpointPorts[service.Name] = append(missingEndpointPorts[service.Name], portName)
			}
		}
		sort.Strings(missingServicePorts[service.Name])
		sort.Strings(missingEndpointPorts[service.Name])
	}
	sort.Strings(serviceNames)
	sort.Strings(missingReadyEndpoints)
	status := model.HealthStatusPass
	message := "all managed services have ready endpoints"
	if len(serviceNames) == 0 || len(missingReadyEndpoints) > 0 || len(missingServicePorts) > 0 || len(missingEndpointPorts) > 0 {
		status = model.HealthStatusFail
		message = "one or more managed services have missing ports or no ready endpoints"
	}
	report.AddCheck("services_endpoints", status, model.HealthSeverityCritical, message, map[string]any{
		"services":               serviceNames,
		"endpointReadyAddresses": endpointReady,
		"missingReadyEndpoints":  missingReadyEndpoints,
		"requiredServerPorts":    requiredServerPorts,
		"missingServicePorts":    missingServicePorts,
		"missingEndpointPorts":   missingEndpointPorts,
	}, started)
}

func (s *Store) checkKeeperRUOK(ctx context.Context, report *model.HealthcheckReport, namespace string, keeperPods []corev1.Pod) {
	started := time.Now()
	if len(keeperPods) == 0 {
		report.AddCheck("keeper_ruok", model.HealthStatusFail, model.HealthSeverityCritical, "no Keeper pods are available for ruok probe", nil, started)
		return
	}
	results := map[string]string{}
	failures := map[string]string{}
	for _, pod := range keeperPods {
		stdout, stderr, err := s.execPod(ctx, namespace, pod.Name, "clickhouse-keeper", []string{"bash", "-lc", "exec 3<>/dev/tcp/127.0.0.1/9181; printf ruok >&3; timeout 2 cat <&3"}, "")
		output := strings.TrimSpace(stdout)
		results[pod.Name] = output
		if err != nil || !strings.Contains(output, "imok") {
			failures[pod.Name] = trimEvidence(stderr + " " + errString(err))
		}
	}
	status := model.HealthStatusPass
	message := "all Keeper pods answered ruok"
	if len(failures) > 0 {
		status = model.HealthStatusFail
		message = "one or more Keeper pods did not answer ruok with imok"
	}
	report.AddCheck("keeper_ruok", status, model.HealthSeverityCritical, message, map[string]any{
		"results":  results,
		"failures": failures,
	}, started)
}

func (s *Store) checkKeeperRoles(ctx context.Context, report *model.HealthcheckReport, namespace string, keeperPods []corev1.Pod) {
	started := time.Now()
	if len(keeperPods) == 0 {
		report.AddCheck("keeper_leader_follower", model.HealthStatusFail, model.HealthSeverityCritical, "no Keeper pods are available for mntr probe", nil, started)
		return
	}
	roles := map[string]string{}
	failures := map[string]string{}
	leaders := 0
	for _, pod := range keeperPods {
		stdout, stderr, err := s.execPod(ctx, namespace, pod.Name, "clickhouse-keeper", []string{"bash", "-lc", "exec 3<>/dev/tcp/127.0.0.1/9181; printf mntr >&3; timeout 2 cat <&3"}, "")
		role := parseKeeperRole(stdout)
		roles[pod.Name] = role
		if role == "leader" {
			leaders++
		}
		if err != nil || (role != "leader" && role != "follower") {
			failures[pod.Name] = trimEvidence(stderr + " " + errString(err))
		}
	}
	status := model.HealthStatusPass
	message := "Keeper quorum has one leader and followers"
	if leaders != 1 || len(failures) > 0 {
		status = model.HealthStatusFail
		message = "Keeper quorum role distribution is invalid"
	}
	report.AddCheck("keeper_leader_follower", status, model.HealthSeverityCritical, message, map[string]any{
		"roles":    roles,
		"leaders":  leaders,
		"failures": failures,
	}, started)
}

func (s *Store) checkClickHouseSelect(ctx context.Context, report *model.HealthcheckReport, namespace string, serverPods []corev1.Pod) {
	started := time.Now()
	pod, ok := firstPod(serverPods)
	if !ok {
		report.AddCheck("clickhouse_select_1", model.HealthStatusFail, model.HealthSeverityCritical, "no ClickHouse pod is available for SQL probe", nil, started)
		return
	}
	stdout, stderr, err := s.clickHouseQuery(ctx, namespace, pod.Name, "SELECT 1 FORMAT TSV")
	if err != nil || strings.TrimSpace(stdout) != "1" {
		report.AddCheck("clickhouse_select_1", model.HealthStatusFail, model.HealthSeverityCritical, "ClickHouse SELECT 1 failed", map[string]any{
			"pod":    pod.Name,
			"stdout": trimEvidence(stdout),
			"stderr": trimEvidence(stderr),
			"error":  errString(err),
		}, started)
		return
	}
	report.AddCheck("clickhouse_select_1", model.HealthStatusPass, model.HealthSeverityCritical, "ClickHouse SQL endpoint accepted SELECT 1", map[string]any{
		"pod":    pod.Name,
		"result": "1",
	}, started)
}

func (s *Store) checkSystemClusters(ctx context.Context, report *model.HealthcheckReport, namespace string, serverPods []corev1.Pod, topology model.Topology) {
	started := time.Now()
	pod, ok := firstPod(serverPods)
	if !ok {
		report.AddCheck("system_clusters_topology", model.HealthStatusFail, model.HealthSeverityCritical, "no ClickHouse pod is available for topology probe", nil, started)
		return
	}
	query := fmt.Sprintf("SELECT shard_num, replica_num, host_name FROM system.clusters WHERE cluster='%s' ORDER BY shard_num, replica_num FORMAT TSV", clickHouseClusterName)
	stdout, stderr, err := s.clickHouseQuery(ctx, namespace, pod.Name, query)
	if err != nil {
		report.AddCheck("system_clusters_topology", model.HealthStatusFail, model.HealthSeverityCritical, "system.clusters query failed", map[string]any{
			"pod":    pod.Name,
			"stderr": trimEvidence(stderr),
			"error":  errString(err),
		}, started)
		return
	}
	rows := parseTSV(stdout)
	shards := map[string]int{}
	for _, row := range rows {
		if len(row) >= 2 {
			shards[row[0]]++
		}
	}
	status := model.HealthStatusPass
	message := "system.clusters matches expected topology"
	if len(rows) != topology.Shards*topology.ReplicasPerShard || len(shards) != topology.Shards {
		status = model.HealthStatusFail
		message = "system.clusters does not match expected topology"
	}
	for _, replicas := range shards {
		if replicas != topology.ReplicasPerShard {
			status = model.HealthStatusFail
			message = "system.clusters replica distribution does not match expected topology"
			break
		}
	}
	report.AddCheck("system_clusters_topology", status, model.HealthSeverityCritical, message, map[string]any{
		"cluster":          clickHouseClusterName,
		"expectedShards":   topology.Shards,
		"expectedReplicas": topology.ReplicasPerShard,
		"actualRows":       len(rows),
		"actualShardCount": len(shards),
		"replicasPerShard": shards,
		"sampleQueryPod":   pod.Name,
	}, started)
}

func (s *Store) checkWriteReadProbe(ctx context.Context, report *model.HealthcheckReport, namespace string, serverPods []corev1.Pod, topology model.Topology) {
	started := time.Now()
	pod, ok := firstPod(serverPods)
	if !ok {
		report.AddCheck("write_read_probe", model.HealthStatusFail, model.HealthSeverityCritical, "no ClickHouse pod is available for write/read probe", nil, started)
		return
	}
	if _, stderr, err := s.clickHouseQuery(ctx, namespace, pod.Name, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s ON CLUSTER %s", healthcheckDatabase, clickHouseClusterName)); err != nil {
		report.AddCheck("write_read_probe", model.HealthStatusFail, model.HealthSeverityCritical, "healthcheck database preparation failed", map[string]any{
			"pod":    pod.Name,
			"stderr": trimEvidence(stderr),
			"error":  errString(err),
		}, started)
		return
	}
	for _, serverPod := range serverPods {
		if err := s.validateHealthcheckTableDefinitions(ctx, namespace, serverPod.Name, true); err != nil {
			report.AddCheck("write_read_probe", model.HealthStatusFail, model.HealthSeverityCritical, "existing healthcheck table definition drift detected before write", map[string]any{
				"pod":   serverPod.Name,
				"error": errString(err),
			}, started)
			return
		}
	}
	stdout, stderr, err := s.clickHouseMultiquery(ctx, namespace, pod.Name, healthcheckCreateTablesSQL())
	if err != nil {
		report.AddCheck("write_read_probe", model.HealthStatusFail, model.HealthSeverityCritical, "healthcheck table creation failed", map[string]any{
			"pod":    pod.Name,
			"stdout": trimEvidence(stdout),
			"stderr": trimEvidence(stderr),
			"error":  errString(err),
		}, started)
		return
	}
	for _, serverPod := range serverPods {
		if err := s.validateHealthcheckTableDefinitions(ctx, namespace, serverPod.Name, false); err != nil {
			report.AddCheck("write_read_probe", model.HealthStatusFail, model.HealthSeverityCritical, "healthcheck table definition drift detected after create", map[string]any{
				"pod":   serverPod.Name,
				"error": errString(err),
			}, started)
			return
		}
	}
	probeRows := healthcheckProbeRowCount(topology)
	stdout, stderr, err = s.clickHouseMultiquery(ctx, namespace, pod.Name, healthcheckWriteSQL(probeRows))
	if err != nil {
		report.AddCheck("write_read_probe", model.HealthStatusFail, model.HealthSeverityCritical, "healthcheck write probe failed", map[string]any{
			"pod":    pod.Name,
			"stdout": trimEvidence(stdout),
			"stderr": trimEvidence(stderr),
			"error":  errString(err),
		}, started)
		return
	}
	for _, serverPod := range serverPods {
		_, _, syncErr := s.clickHouseQuery(ctx, namespace, serverPod.Name, fmt.Sprintf("SYSTEM SYNC REPLICA %s.%s", healthcheckDatabase, healthcheckLocalTable))
		if syncErr != nil {
			report.AddCheck("write_read_probe", model.HealthStatusFail, model.HealthSeverityCritical, "replica sync failed after write probe", map[string]any{
				"pod":   serverPod.Name,
				"error": errString(syncErr),
			}, started)
			return
		}
	}
	for _, assertion := range healthcheckWriteAssertions(topology, probeRows) {
		if _, stderr, err := s.clickHouseQuery(ctx, namespace, pod.Name, assertion); err != nil {
			report.AddCheck("write_read_probe", model.HealthStatusFail, model.HealthSeverityCritical, "write/read assertion failed", map[string]any{
				"pod":       pod.Name,
				"assertion": assertion,
				"stderr":    trimEvidence(stderr),
				"error":     errString(err),
			}, started)
			return
		}
	}
	report.AddCheck("write_read_probe", model.HealthStatusPass, model.HealthSeverityCritical, "Distributed write/read probe succeeded through reserved healthcheck tables", map[string]any{
		"pod":              pod.Name,
		"database":         healthcheckDatabase,
		"localTable":       healthcheckLocalTable,
		"distributedTable": healthcheckDistributedTable,
		"insertedRows":     probeRows,
	}, started)
}

func (s *Store) checkReplicaHealth(ctx context.Context, report *model.HealthcheckReport, namespace string, serverPods []corev1.Pod, topology model.Topology) {
	started := time.Now()
	pod, ok := firstPod(serverPods)
	if !ok {
		report.AddCheck("replica_health", model.HealthStatusFail, model.HealthSeverityCritical, "no ClickHouse pod is available for replica health probe", nil, started)
		return
	}
	query := fmt.Sprintf("SELECT hostName(), replica_name, total_replicas, active_replicas, is_readonly, is_session_expired, queue_size FROM clusterAllReplicas('%s', system.replicas) WHERE database='%s' AND table='%s' ORDER BY hostName() FORMAT TSV", clickHouseClusterName, healthcheckDatabase, healthcheckLocalTable)
	stdout, stderr, err := s.clickHouseQuery(ctx, namespace, pod.Name, query)
	if err != nil {
		report.AddCheck("replica_health", model.HealthStatusFail, model.HealthSeverityCritical, "replica health query failed", map[string]any{
			"pod":    pod.Name,
			"stderr": trimEvidence(stderr),
			"error":  errString(err),
		}, started)
		return
	}
	rows := parseTSV(stdout)
	unhealthy := []map[string]any{}
	for _, row := range rows {
		if len(row) < 7 {
			unhealthy = append(unhealthy, map[string]any{"row": row, "reason": "unexpected column count"})
			continue
		}
		total, _ := strconv.Atoi(row[2])
		active, _ := strconv.Atoi(row[3])
		readOnly, _ := strconv.Atoi(row[4])
		sessionExpired, _ := strconv.Atoi(row[5])
		queueSize, _ := strconv.Atoi(row[6])
		if total != topology.ReplicasPerShard || active != topology.ReplicasPerShard || readOnly != 0 || sessionExpired != 0 || queueSize != 0 {
			unhealthy = append(unhealthy, map[string]any{
				"host":             row[0],
				"replica":          row[1],
				"totalReplicas":    total,
				"activeReplicas":   active,
				"isReadonly":       readOnly,
				"isSessionExpired": sessionExpired,
				"queueSize":        queueSize,
			})
		}
	}
	status := model.HealthStatusPass
	message := "all healthcheck table replicas are active and caught up"
	if len(rows) != topology.Shards*topology.ReplicasPerShard || len(unhealthy) > 0 {
		status = model.HealthStatusFail
		message = "one or more healthcheck table replicas are unhealthy"
	}
	report.AddCheck("replica_health", status, model.HealthSeverityCritical, message, map[string]any{
		"rows":              len(rows),
		"expectedRows":      topology.Shards * topology.ReplicasPerShard,
		"replicasPerShard":  topology.ReplicasPerShard,
		"unhealthyReplicas": unhealthy,
	}, started)
}

func (s *Store) checkMetricsEndpoint(ctx context.Context, report *model.HealthcheckReport, namespace string, serverPods []corev1.Pod) {
	started := time.Now()
	pod, ok := firstPod(serverPods)
	if !ok {
		report.AddCheck("metrics_endpoint", model.HealthStatusWarn, model.HealthSeverityWarning, "no ClickHouse pod is available for metrics probe", nil, started)
		return
	}
	stdout, stderr, err := s.execPod(ctx, namespace, pod.Name, "clickhouse", []string{"bash", "-lc", "curl -fsS --max-time 5 http://127.0.0.1:9363/metrics | head -n 5"}, "")
	if err != nil || !strings.Contains(stdout, "#") {
		report.AddCheck("metrics_endpoint", model.HealthStatusWarn, model.HealthSeverityWarning, "ClickHouse metrics endpoint is not reachable or not Prometheus-formatted", map[string]any{
			"pod":    pod.Name,
			"stdout": trimEvidence(stdout),
			"stderr": trimEvidence(stderr),
			"error":  errString(err),
		}, started)
		return
	}
	report.AddCheck("metrics_endpoint", model.HealthStatusPass, model.HealthSeverityWarning, "ClickHouse metrics endpoint returned Prometheus text", map[string]any{
		"pod":    pod.Name,
		"sample": trimEvidence(stdout),
	}, started)
}

func addFailure(report *model.HealthcheckReport, name, severity, message string, err error, started time.Time) {
	report.AddCheck(name, model.HealthStatusFail, severity, message, map[string]any{
		"error": errString(err),
	}, started)
}

func (s *Store) collectHealthcheckSensitiveValues(ctx context.Context, namespace, name string) ([]string, error) {
	server, err := s.dynamic.Resource(unitSetGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("read server UnitSet for secret redaction: %w", err)
	}
	secretNames := []string{server.GetAnnotations()[adminSecretAnnotation], "aes-secret-key"}
	values := []string{}
	for _, secretName := range secretNames {
		if secretName == "" {
			return nil, fmt.Errorf("required secret reference is missing")
		}
		secret, err := s.core.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("read secret material for response redaction: %w", err)
		}
		for _, value := range secret.Data {
			if len(value) < 4 {
				continue
			}
			values = append(values, string(value), base64.StdEncoding.EncodeToString(value))
		}
	}
	return values, nil
}

func redactHealthcheckReport(report *model.HealthcheckReport, sensitiveValues []string) {
	for index := range report.Checks {
		report.Checks[index].Message = redactString(report.Checks[index].Message, sensitiveValues)
		report.Checks[index].Evidence = redactMap(report.Checks[index].Evidence, sensitiveValues)
	}
}

func addSecretLeakageCheck(report *model.HealthcheckReport, sensitiveValues []string, sensitiveErr error) {
	started := time.Now()
	if sensitiveErr != nil {
		report.AddCheck("no_secret_leakage", model.HealthStatusFail, model.HealthSeverityCritical, "secret material could not be loaded for response leakage validation", nil, started)
		return
	}
	payload, err := json.Marshal(report)
	if err != nil {
		report.AddCheck("no_secret_leakage", model.HealthStatusFail, model.HealthSeverityCritical, "healthcheck report could not be marshaled for secret scan", map[string]any{
			"error": errString(err),
		}, started)
		return
	}
	lower := strings.ToLower(string(payload))
	forbidden := []string{"clickhouse_admin_password", "aes_secret_key", "secretkeyref"}
	matches := []string{}
	for _, pattern := range forbidden {
		if strings.Contains(lower, pattern) {
			matches = append(matches, pattern)
		}
	}
	for _, sensitiveValue := range sensitiveValues {
		if sensitiveValue != "" && strings.Contains(string(payload), sensitiveValue) {
			matches = append(matches, "secret-value")
			break
		}
	}
	status := model.HealthStatusPass
	message := "healthcheck report does not include secret field names or values collected by the API server"
	if len(matches) > 0 {
		status = model.HealthStatusFail
		message = "healthcheck report contains forbidden secret-like tokens"
	}
	report.AddCheck("no_secret_leakage", status, model.HealthSeverityCritical, message, map[string]any{
		"forbiddenMatches": matches,
	}, started)
}

func redactMap(input map[string]any, sensitiveValues []string) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = redactValue(value, sensitiveValues)
	}
	return output
}

func redactValue(value any, sensitiveValues []string) any {
	switch typed := value.(type) {
	case string:
		return redactString(typed, sensitiveValues)
	case []string:
		output := make([]string, len(typed))
		for index := range typed {
			output[index] = redactString(typed[index], sensitiveValues)
		}
		return output
	case []any:
		output := make([]any, len(typed))
		for index := range typed {
			output[index] = redactValue(typed[index], sensitiveValues)
		}
		return output
	case map[string]string:
		output := make(map[string]string, len(typed))
		for key, item := range typed {
			output[key] = redactString(item, sensitiveValues)
		}
		return output
	case map[string]any:
		return redactMap(typed, sensitiveValues)
	default:
		return value
	}
}

func redactString(value string, sensitiveValues []string) string {
	for _, sensitiveValue := range sensitiveValues {
		if sensitiveValue != "" {
			value = strings.ReplaceAll(value, sensitiveValue, "[REDACTED]")
		}
	}
	return value
}

func (s *Store) clickHouseQuery(ctx context.Context, namespace, podName, query string) (string, string, error) {
	return s.execPod(ctx, namespace, podName, "clickhouse", []string{"service-ctl.sh", "login", "--query", query}, "")
}

func (s *Store) clickHouseMultiquery(ctx context.Context, namespace, podName, sql string) (string, string, error) {
	return s.execPod(ctx, namespace, podName, "clickhouse", []string{"bash", "-lc", "service-ctl.sh login --multiquery"}, sql)
}

func (s *Store) execPod(ctx context.Context, namespace, podName, container string, command []string, stdin string) (string, string, error) {
	if s.restConfig == nil {
		return "", "", fmt.Errorf("Kubernetes REST config is not available for pod exec")
	}
	request := s.core.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdin:     stdin != "",
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(s.restConfig, http.MethodPost, request.URL())
	if err != nil {
		return "", "", fmt.Errorf("create pod exec executor: %w", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var stdinReader io.Reader
	if stdin != "" {
		stdinReader = strings.NewReader(stdin)
	}
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  stdinReader,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	if err != nil {
		return stdout.String(), stderr.String(), fmt.Errorf("exec %s/%s container %s: %w", namespace, podName, container, err)
	}
	return stdout.String(), stderr.String(), nil
}

func healthcheckCreateTablesSQL() string {
	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %[1]s.%[3]s ON CLUSTER %[2]s
(
    id UInt64,
    shard_key UInt64,
    message String
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{cluster}/{shard}/%[1]s/%[3]s', '{replica}')
ORDER BY id;
CREATE TABLE IF NOT EXISTS %[1]s.%[4]s ON CLUSTER %[2]s
AS %[1]s.%[3]s
ENGINE = Distributed(%[2]s, %[1]s, %[3]s, shard_key);
`, healthcheckDatabase, clickHouseClusterName, healthcheckLocalTable, healthcheckDistributedTable)
}

func healthcheckWriteSQL(rows int) string {
	values := make([]string, 0, rows)
	for index := 1; index <= rows; index++ {
		values = append(values, fmt.Sprintf("(%d, %d, 'healthcheck-%d')", index, index, index))
	}
	return fmt.Sprintf(`
SET insert_distributed_sync = 1;
TRUNCATE TABLE %[1]s.%[3]s ON CLUSTER %[2]s;
INSERT INTO %[1]s.%[4]s VALUES
    %[5]s;
`, healthcheckDatabase, clickHouseClusterName, healthcheckLocalTable, healthcheckDistributedTable, strings.Join(values, ",\n    "))
}

func healthcheckProbeRowCount(topology model.Topology) int {
	rows := topology.Shards * 4
	if rows < 8 {
		return 8
	}
	return rows
}

func healthcheckWriteAssertions(topology model.Topology, probeRows int) []string {
	return []string{
		fmt.Sprintf("SELECT throwIf(count() != %d, 'Distributed row count does not match inserted probe rows') FROM %s.%s", probeRows, healthcheckDatabase, healthcheckDistributedTable),
		fmt.Sprintf("SELECT throwIf(count() != %d, 'Distributed table must read every shard') FROM (SELECT _shard_num FROM %s.%s GROUP BY _shard_num)", topology.Shards, healthcheckDatabase, healthcheckDistributedTable),
		fmt.Sprintf("SELECT throwIf(count() != %d OR sum(replicas != %d OR row_count_variants != 1 OR min_rows = 0) != 0, 'Replica row distribution is inconsistent') FROM (SELECT shard, count() AS replicas, uniqExact(local_rows) AS row_count_variants, min(local_rows) AS min_rows FROM (SELECT getMacro('shard') AS shard, getMacro('replica') AS replica, count() AS local_rows FROM clusterAllReplicas('%s', %s.%s) GROUP BY shard, replica) GROUP BY shard)", topology.Shards, topology.ReplicasPerShard, clickHouseClusterName, healthcheckDatabase, healthcheckLocalTable),
	}
}

func (s *Store) validateHealthcheckTableDefinitions(ctx context.Context, namespace, podName string, allowMissing bool) error {
	tables := []struct {
		name     string
		validate func(string) bool
	}{
		{name: healthcheckLocalTable, validate: validHealthcheckLocalDefinition},
		{name: healthcheckDistributedTable, validate: validHealthcheckDistributedDefinition},
	}
	for _, table := range tables {
		exists, stderr, err := s.clickHouseQuery(ctx, namespace, podName, fmt.Sprintf("EXISTS TABLE %s.%s FORMAT TSV", healthcheckDatabase, table.name))
		if err != nil {
			return fmt.Errorf("check healthcheck table %s existence: %s %w", table.name, trimEvidence(stderr), err)
		}
		if strings.TrimSpace(exists) == "0" && allowMissing {
			continue
		}
		if strings.TrimSpace(exists) != "1" {
			return fmt.Errorf("healthcheck table %s does not exist after create", table.name)
		}
		definition, stderr, err := s.clickHouseQuery(ctx, namespace, podName, fmt.Sprintf("SHOW CREATE TABLE %s.%s FORMAT TSVRaw", healthcheckDatabase, table.name))
		if err != nil {
			return fmt.Errorf("read healthcheck table %s definition: %s %w", table.name, trimEvidence(stderr), err)
		}
		if !table.validate(definition) {
			return fmt.Errorf("healthcheck table %s definition differs from the API-server-owned schema", table.name)
		}
	}
	return nil
}

func validHealthcheckLocalDefinition(definition string) bool {
	return hasColumnDefinition(definition, "id", "UInt64") &&
		hasColumnDefinition(definition, "shard_key", "UInt64") &&
		hasColumnDefinition(definition, "message", "String") &&
		strings.Contains(definition, "ReplicatedMergeTree") &&
		strings.Contains(definition, "/upm_healthcheck/local_events") &&
		strings.Contains(definition, "ORDER BY id")
}

func validHealthcheckDistributedDefinition(definition string) bool {
	return strings.Contains(definition, "Distributed") &&
		strings.Contains(definition, clickHouseClusterName) &&
		strings.Contains(definition, healthcheckDatabase) &&
		strings.Contains(definition, healthcheckLocalTable) &&
		strings.Contains(definition, "shard_key")
}

func podIsReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func podNames(pods []corev1.Pod) []string {
	names := make([]string, 0, len(pods))
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	return names
}

func firstPod(pods []corev1.Pod) (corev1.Pod, bool) {
	if len(pods) == 0 {
		return corev1.Pod{}, false
	}
	return pods[0], true
}

func storageResourceNameMatches(name, clusterName string) bool {
	return resourceNameMatches(name, clusterName) || strings.Contains(name, clusterName+"-")
}

func parseKeeperRole(output string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "zk_server_state" {
			return fields[1]
		}
	}
	return ""
}

func hasColumnDefinition(createStatement, column, dataType string) bool {
	return strings.Contains(createStatement, column+" "+dataType) ||
		strings.Contains(createStatement, "`"+column+"` "+dataType)
}

func parseTSV(output string) [][]string {
	rows := [][]string{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rows = append(rows, strings.Split(line, "\t"))
	}
	return rows
}

func trimEvidence(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1024 {
		return value[:1024] + "...[truncated]"
	}
	return value
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return trimEvidence(err.Error())
}

func (s *Store) ensureNamespace(ctx context.Context, name string) error {
	_, err := s.core.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return internalError("get Namespace", err)
	}
	_, err = s.core.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return internalError("create Namespace", err)
	}
	return nil
}

func (s *Store) ensureProject(ctx context.Context, name string) error {
	_, err := s.dynamic.Resource(projectGVR).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return internalError("get UPMIO Project", err)
	}
	project := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "upm.syntropycloud.io/v1alpha2",
		"kind":       "Project",
		"metadata": map[string]any{
			"name": name,
		},
		"spec": map[string]any{},
	}}
	_, err = s.dynamic.Resource(projectGVR).Create(ctx, project, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return internalError("create UPMIO Project", err)
	}
	return nil
}

func (s *Store) validatePackagePrerequisites(ctx context.Context, request model.CreateClusterRequest) error {
	requiredConfigMaps := []string{
		fmt.Sprintf("clickhouse-%s-config-template", request.Version),
		fmt.Sprintf("clickhouse-%s-config-value", request.Version),
		fmt.Sprintf("clickhouse-keeper-%s-config-template", request.Version),
		fmt.Sprintf("clickhouse-keeper-%s-config-value", request.Version),
	}
	for _, name := range requiredConfigMaps {
		if _, err := s.core.CoreV1().ConfigMaps(managerNamespace).Get(ctx, name, metav1.GetOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				return &model.APIError{
					Status:  http.StatusUnprocessableEntity,
					Code:    "PACKAGE_VERSION_NOT_INSTALLED",
					Message: fmt.Sprintf("required package ConfigMap %s is not installed in %s", name, managerNamespace),
				}
			}
			return internalError("check package ConfigMap", err)
		}
	}
	requiredPodTemplates := []string{
		fmt.Sprintf("clickhouse-%s", request.Version),
		fmt.Sprintf("clickhouse-keeper-%s", request.Version),
	}
	for _, name := range requiredPodTemplates {
		if _, err := s.core.CoreV1().PodTemplates(managerNamespace).Get(ctx, name, metav1.GetOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				return &model.APIError{
					Status:  http.StatusUnprocessableEntity,
					Code:    "PACKAGE_VERSION_NOT_INSTALLED",
					Message: fmt.Sprintf("required package PodTemplate %s is not installed in %s", name, managerNamespace),
				}
			}
			return internalError("check package PodTemplate", err)
		}
	}

	installed, err := s.installedPackageTopology(ctx, request.Version)
	if err != nil {
		return err
	}
	if installed != request.Topology {
		return &model.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "PACKAGE_TOPOLOGY_MISMATCH",
			Message: "requested topology must match the installed ClickHouse package topology",
			Details: map[string]any{
				"requested": request.Topology,
				"installed": installed,
			},
		}
	}
	return nil
}

func (s *Store) summaryFromServerUnitSet(ctx context.Context, server *unstructured.Unstructured) (model.ClusterSummary, error) {
	name := server.GetLabels()[serviceGroupNameLabel]
	if name == "" {
		name = server.GetName()
	}
	namespace := server.GetNamespace()
	keeper, err := s.dynamic.Resource(unitSetGVR).Namespace(namespace).Get(ctx, name+"-keeper", metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return model.ClusterSummary{}, internalError("get Keeper UnitSet", err)
	}

	version, _, _ := unstructured.NestedString(server.Object, "spec", "version")
	topology, err := s.installedPackageTopology(ctx, version)
	if err != nil {
		topology = model.Topology{Shards: 1, ReplicasPerShard: 1}
	}
	topology.Shards = annotationInt(server, topologyShardsAnnotation, topology.Shards)
	topology.ReplicasPerShard = annotationInt(server, topologyReplicasPerShardAnnotation, topology.ReplicasPerShard)
	topology.KeeperReplicas = annotationInt(server, topologyKeeperReplicasAnnotation, topology.KeeperReplicas)

	serverExpected := nestedInt64Or(server.Object, int64(topology.Shards*topology.ReplicasPerShard), "spec", "units")
	serverReady := nestedInt64Or(server.Object, 0, "status", "readyUnits")
	keeperExpected := int64(topology.KeeperReplicas)
	keeperReady := int64(0)
	if err == nil {
		keeperExpected = nestedInt64Or(keeper.Object, keeperExpected, "spec", "units")
		keeperReady = nestedInt64Or(keeper.Object, 0, "status", "readyUnits")
	}
	status := "Provisioning"
	if serverReady == serverExpected && keeperExpected > 0 && keeperReady == keeperExpected {
		status = "Running"
	} else if serverReady > 0 || keeperReady > 0 {
		status = "Degraded"
	}
	return model.ClusterSummary{
		Namespace: namespace,
		Name:      name,
		Version:   version,
		Status:    status,
		Topology:  topology,
		Ready: model.ReadySummary{
			Keeper: fmt.Sprintf("%d/%d", keeperReady, keeperExpected),
			Server: fmt.Sprintf("%d/%d", serverReady, serverExpected),
		},
		Resources: model.ClusterResourceRef{
			Project:       namespace,
			KeeperUnitSet: name + "-keeper",
			ServerUnitSet: server.GetName(),
		},
	}, nil
}

func (s *Store) installedPackageTopology(ctx context.Context, version string) (model.Topology, error) {
	configMap, err := s.core.CoreV1().ConfigMaps(managerNamespace).Get(
		ctx,
		fmt.Sprintf("clickhouse-%s-config-value", version),
		metav1.GetOptions{},
	)
	if err != nil {
		return model.Topology{}, internalError("read ClickHouse package topology", err)
	}
	var installed struct {
		Topology struct {
			Shards           any `yaml:"shards"`
			ReplicasPerShard any `yaml:"replicasPerShard"`
		} `yaml:"topology"`
		Keeper struct {
			Replicas any `yaml:"replicas"`
		} `yaml:"keeper"`
	}
	if err := yaml.Unmarshal([]byte(configMap.Data["clickhouse"]), &installed); err != nil {
		return model.Topology{}, &model.APIError{
			Status:  http.StatusUnprocessableEntity,
			Code:    "PACKAGE_TOPOLOGY_INVALID",
			Message: "installed ClickHouse package topology cannot be parsed",
			Err:     err,
		}
	}
	shards, err := topologyInt(installed.Topology.Shards)
	if err != nil {
		return model.Topology{}, packageTopologyError("topology.shards", err)
	}
	replicasPerShard, err := topologyInt(installed.Topology.ReplicasPerShard)
	if err != nil {
		return model.Topology{}, packageTopologyError("topology.replicasPerShard", err)
	}
	keeperReplicas, err := topologyInt(installed.Keeper.Replicas)
	if err != nil {
		return model.Topology{}, packageTopologyError("keeper.replicas", err)
	}
	return model.Topology{Shards: shards, ReplicasPerShard: replicasPerShard, KeeperReplicas: keeperReplicas}, nil
}

func topologyInt(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case float64:
		return int(typed), nil
	case string:
		return strconv.Atoi(typed)
	default:
		return 0, fmt.Errorf("unsupported value type %T", value)
	}
}

func packageTopologyError(field string, err error) error {
	return &model.APIError{
		Status:  http.StatusUnprocessableEntity,
		Code:    "PACKAGE_TOPOLOGY_INVALID",
		Message: fmt.Sprintf("installed ClickHouse package field %s is invalid", field),
		Err:     err,
	}
}

func keeperUnitSet(request model.CreateClusterRequest) *unstructured.Unstructured {
	return unitSet(request, request.Name+"-keeper", "clickhouse-keeper", request.Topology.KeeperReplicas, request.Storage.KeeperDataSize, false)
}

func serverUnitSet(request model.CreateClusterRequest) *unstructured.Unstructured {
	object := unitSet(request, request.Name, "clickhouse", request.Topology.Shards*request.Topology.ReplicasPerShard, request.Storage.ServerDataSize, request.Monitoring.Enabled)
	object.SetAnnotations(clusterAnnotations(request))
	return object
}

func unitSet(request model.CreateClusterRequest, name, serviceType string, units int, dataSize string, monitoring bool) *unstructured.Unstructured {
	labels := map[string]any{
		serviceGroupNameLabel: request.Name,
		serviceGroupTypeLabel: "clickhouse-sg",
		serviceTypeLabel:      serviceType,
	}
	env := []any{
		map[string]any{"name": "SECRET_MOUNT", "value": "/etc/upm/secret"},
		map[string]any{
			"name": "AES_SECRET_KEY",
			"valueFrom": map[string]any{
				"secretKeyRef": map[string]any{"name": "aes-secret-key", "key": "AES_SECRET_KEY"},
			},
		},
	}
	if serviceType == "clickhouse" {
		env = append(env,
			map[string]any{"name": "SECRET_NAME", "value": request.Security.AdminSecretRef},
			map[string]any{"name": "CLICKHOUSE_ADMIN_SECRET_NAME", "value": request.Security.AdminSecretRef},
			map[string]any{"name": "CLICKHOUSE_ADMIN_PASSWORD_KEY", "value": "CLICKHOUSE_ADMIN_PASSWORD"},
			map[string]any{"name": "CLICKHOUSE_ADMIN_USER", "value": "admin"},
			map[string]any{"name": "CLICKHOUSE_METRICS_PORT", "value": "9363"},
			map[string]any{"name": "CLICKHOUSE_METRICS_PATH", "value": "/metrics"},
		)
	}

	spec := map[string]any{
		"type":    serviceType,
		"version": request.Version,
		"units":   int64(units),
		"env":     env,
		"storage": []any{
			map[string]any{
				"name":             "data",
				"size":             dataSize,
				"storageClassName": request.Storage.ClassName,
				"mountPath":        "/var/lib/" + serviceType,
			},
		},
		"emptyDir": []any{
			map[string]any{
				"name":      "log",
				"size":      "1Gi",
				"mountPath": "/var/log/" + serviceType,
			},
		},
		"extraVolume": []any{
			map[string]any{
				"volume": map[string]any{
					"name": "unit-secret",
					"secret": map[string]any{
						"secretName": request.Security.AdminSecretRef,
					},
				},
				"volumeMountPath": "/etc/upm/secret",
			},
		},
		"unitService": map[string]any{"type": "ClusterIP"},
	}
	if serviceType == "clickhouse" {
		spec["resources"] = map[string]any{
			"requests": map[string]any{"cpu": "500m", "memory": "1Gi"},
			"limits":   map[string]any{"cpu": "2", "memory": "4Gi"},
		}
	} else {
		spec["resources"] = map[string]any{
			"requests": map[string]any{"cpu": "250m", "memory": "512Mi"},
			"limits":   map[string]any{"cpu": "1", "memory": "2Gi"},
		}
	}
	if monitoring {
		spec["podMonitor"] = map[string]any{
			"enable": true,
			"endpoints": []any{
				map[string]any{"port": "metrics"},
			},
		}
	}

	metadata := map[string]any{
		"name":      name,
		"namespace": request.Namespace,
		"labels":    labels,
	}
	if serviceType == "clickhouse" {
		metadata["annotations"] = map[string]any{
			keeperServiceNameAnnotation: request.Name + "-keeper",
		}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "upm.syntropycloud.io/v1alpha2",
		"kind":       "UnitSet",
		"metadata":   metadata,
		"spec":       spec,
	}}
}

func clusterAnnotations(request model.CreateClusterRequest) map[string]string {
	return map[string]string{
		keeperServiceNameAnnotation:        request.Name + "-keeper",
		topologyShardsAnnotation:           strconv.Itoa(request.Topology.Shards),
		topologyReplicasPerShardAnnotation: strconv.Itoa(request.Topology.ReplicasPerShard),
		topologyKeeperReplicasAnnotation:   strconv.Itoa(request.Topology.KeeperReplicas),
		adminSecretAnnotation:              request.Security.AdminSecretRef,
		storageClassAnnotation:             request.Storage.ClassName,
		serverDataSizeAnnotation:           request.Storage.ServerDataSize,
		keeperDataSizeAnnotation:           request.Storage.KeeperDataSize,
		monitoringEnabledAnnotation:        strconv.FormatBool(request.Monitoring.Enabled),
	}
}

func annotationInt(object metav1.Object, key string, defaultValue int) int {
	value := object.GetAnnotations()[key]
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func nestedInt64Or(object map[string]any, defaultValue int64, fields ...string) int64 {
	value, found, err := unstructured.NestedInt64(object, fields...)
	if err != nil || !found {
		return defaultValue
	}
	return value
}

func resourceMatches(name string, resourceLabels map[string]string, clusterName string) bool {
	return resourceLabels[serviceGroupNameLabel] == clusterName || resourceNameMatches(name, clusterName)
}

func resourceNameMatches(name, clusterName string) bool {
	return name == clusterName || strings.HasPrefix(name, clusterName+"-")
}

func unstructuredSummary(object *unstructured.Unstructured, kind string) model.ResourceSummary {
	status, _, _ := unstructured.NestedString(object.Object, "status", "phase")
	if status == "" {
		status, _, _ = unstructured.NestedString(object.Object, "status", "processState")
	}
	details := map[string]any{}
	if expected := nestedInt64Or(object.Object, -1, "status", "expectedUnits"); expected >= 0 {
		details["expectedUnits"] = expected
		details["currentUnits"] = nestedInt64Or(object.Object, 0, "status", "currentUnits")
		details["readyUnits"] = nestedInt64Or(object.Object, 0, "status", "readyUnits")
	}
	return model.ResourceSummary{
		Name:      object.GetName(),
		Namespace: object.GetNamespace(),
		Kind:      kind,
		Status:    status,
		Details:   details,
		Labels:    object.GetLabels(),
	}
}

func podSummary(pod *corev1.Pod) model.ResourceSummary {
	ready := false
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			ready = condition.Status == corev1.ConditionTrue
			break
		}
	}
	return model.ResourceSummary{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Kind:      "Pod",
		Status:    string(pod.Status.Phase),
		Labels:    pod.Labels,
		Details: map[string]any{
			"ready":    ready,
			"nodeName": pod.Spec.NodeName,
			"podIP":    pod.Status.PodIP,
		},
	}
}

func pvcSummary(pvc *corev1.PersistentVolumeClaim) model.ResourceSummary {
	details := map[string]any{}
	if pvc.Spec.StorageClassName != nil {
		details["storageClassName"] = *pvc.Spec.StorageClassName
	}
	if quantity, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
		details["capacity"] = quantity.String()
	}
	return model.ResourceSummary{
		Name:      pvc.Name,
		Namespace: pvc.Namespace,
		Kind:      "PersistentVolumeClaim",
		Status:    string(pvc.Status.Phase),
		Labels:    pvc.Labels,
		Details:   details,
	}
}

func serviceSummary(service *corev1.Service) model.ResourceSummary {
	ports := make([]map[string]any, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		ports = append(ports, map[string]any{
			"name": port.Name,
			"port": port.Port,
		})
	}
	return model.ResourceSummary{
		Name:      service.Name,
		Namespace: service.Namespace,
		Kind:      "Service",
		Status:    "Available",
		Labels:    service.Labels,
		Details: map[string]any{
			"type":      service.Spec.Type,
			"clusterIP": service.Spec.ClusterIP,
			"ports":     ports,
		},
	}
}

func endpointSummary(endpoint *corev1.Endpoints) model.ResourceSummary {
	readyAddresses := 0
	notReadyAddresses := 0
	for _, subset := range endpoint.Subsets {
		readyAddresses += len(subset.Addresses)
		notReadyAddresses += len(subset.NotReadyAddresses)
	}
	status := "Ready"
	if readyAddresses == 0 {
		status = "NotReady"
	}
	return model.ResourceSummary{
		Name:      endpoint.Name,
		Namespace: endpoint.Namespace,
		Kind:      "Endpoints",
		Status:    status,
		Labels:    endpoint.Labels,
		Details: map[string]any{
			"readyAddresses":    readyAddresses,
			"notReadyAddresses": notReadyAddresses,
		},
	}
}

func eventSummary(event *corev1.Event) model.ResourceSummary {
	return model.ResourceSummary{
		Name:      event.Name,
		Namespace: event.Namespace,
		Kind:      "Event",
		Status:    event.Type,
		Details: map[string]any{
			"reason":         event.Reason,
			"involvedObject": event.InvolvedObject.Name,
		},
	}
}

func internalError(action string, err error) error {
	var apiStatus apierrors.APIStatus
	if errors.As(err, &apiStatus) && apiStatus.Status().Code == http.StatusForbidden {
		return &model.APIError{
			Status:  http.StatusForbidden,
			Code:    "KUBERNETES_FORBIDDEN",
			Message: "upm-api-server is not authorized to access the required Kubernetes resource",
			Err:     fmt.Errorf("%s: %w", action, err),
		}
	}
	return &model.APIError{
		Status:  http.StatusInternalServerError,
		Code:    "KUBERNETES_API_ERROR",
		Message: "Kubernetes API operation failed",
		Err:     fmt.Errorf("%s: %w", action, err),
	}
}

func notFoundError(namespace, name string) error {
	return &model.APIError{
		Status:  http.StatusNotFound,
		Code:    "CLUSTER_NOT_FOUND",
		Message: "managed cluster not found",
		Details: map[string]any{"namespace": namespace, "name": name},
	}
}
