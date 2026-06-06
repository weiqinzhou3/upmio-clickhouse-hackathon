package kube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/model"
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
)

type Store struct {
	dynamic dynamic.Interface
	core    kubernetes.Interface
}

func NewInClusterStore() (*Store, error) {
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
	return &Store{dynamic: dynamicClient, core: coreClient}, nil
}

func NewStore(dynamicClient dynamic.Interface, coreClient kubernetes.Interface) *Store {
	return &Store{dynamic: dynamicClient, core: coreClient}
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
