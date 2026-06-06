package kube

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/model"
	promclient "github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

type fakePrometheusQuerier struct {
	results map[string][]promclient.Sample
	errors  map[string]error
	queries []string
}

func (f *fakePrometheusQuerier) Query(_ context.Context, query string) ([]promclient.Sample, error) {
	f.queries = append(f.queries, query)
	if err := f.errors[query]; err != nil {
		return nil, err
	}
	return f.results[query], nil
}

func TestUnitSetRendering(t *testing.T) {
	request := model.CreateClusterRequest{
		Namespace: "upm-clickhouse",
		Name:      "ch-demo",
		Version:   "26.3.9.8",
		Topology: model.Topology{
			Shards:           2,
			ReplicasPerShard: 2,
			KeeperReplicas:   3,
		},
		Storage: model.Storage{
			ClassName:      "local-path",
			ServerDataSize: "20Gi",
			KeeperDataSize: "10Gi",
		},
		Security:   model.Security{AdminSecretRef: "ch-demo-secret"},
		Monitoring: model.MonitoringConfig{Enabled: true},
	}

	server := serverUnitSet(request)
	keeper := keeperUnitSet(request)

	serverUnits := nestedInt64Or(server.Object, 0, "spec", "units")
	keeperUnits := nestedInt64Or(keeper.Object, 0, "spec", "units")
	if serverUnits != 4 || keeperUnits != 3 {
		t.Fatalf("unexpected units: server=%d keeper=%d", serverUnits, keeperUnits)
	}
	if server.GetAnnotations()[topologyShardsAnnotation] != "2" {
		t.Fatalf("server topology annotation missing")
	}
	if server.GetAnnotations()[keeperServiceNameAnnotation] != "ch-demo-keeper" {
		t.Fatalf("keeper service annotation missing")
	}

	env, found, err := unstructured.NestedSlice(server.Object, "spec", "env")
	if err != nil || !found {
		t.Fatalf("read rendered environment: found=%v err=%v", found, err)
	}
	for _, item := range env {
		entry := item.(map[string]any)
		if entry["name"] == "CLICKHOUSE_ADMIN_PASSWORD" && entry["value"] != nil {
			t.Fatalf("rendered UnitSet must not contain password values")
		}
	}
}

func TestInstalledPackageTopology(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clickhouse-26.3.9.8-config-value",
			Namespace: managerNamespace,
		},
		Data: map[string]string{
			"clickhouse": "topology:\n  shards: \"2\"\n  replicasPerShard: \"2\"\nkeeper:\n  replicas: \"3\"\n",
		},
	})
	store := &Store{core: client}

	topology, err := store.installedPackageTopology(context.Background(), "26.3.9.8")
	if err != nil {
		t.Fatalf("read installed topology: %v", err)
	}
	expected := model.Topology{Shards: 2, ReplicasPerShard: 2, KeeperReplicas: 3}
	if topology != expected {
		t.Fatalf("expected %#v, got %#v", expected, topology)
	}
}

func TestValidateSecretKey(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "admin", Namespace: "upm-clickhouse"},
		Data:       map[string][]byte{"CLICKHOUSE_ADMIN_PASSWORD": []byte("redacted-test-value")},
	})
	store := &Store{core: client}

	if err := store.validateSecretKey(context.Background(), "upm-clickhouse", "admin", "CLICKHOUSE_ADMIN_PASSWORD", "ADMIN_SECRET"); err != nil {
		t.Fatalf("expected secret key validation to pass: %v", err)
	}
	if err := store.validateSecretKey(context.Background(), "upm-clickhouse", "admin", "missing", "ADMIN_SECRET"); err == nil {
		t.Fatalf("expected missing key validation error")
	}
}

func TestResourceNameMatchesClusterBoundary(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		want        bool
	}{
		{name: "clickhouse-demo", clusterName: "clickhouse-demo", want: true},
		{name: "clickhouse-demo-0", clusterName: "clickhouse-demo", want: true},
		{name: "clickhouse-demo-keeper-0", clusterName: "clickhouse-demo", want: true},
		{name: "clickhouse-demo2-0", clusterName: "clickhouse-demo", want: false},
		{name: "clickhouse-demonstration-0", clusterName: "clickhouse-demo", want: false},
	}

	for _, test := range tests {
		if got := resourceNameMatches(test.name, test.clusterName); got != test.want {
			t.Fatalf("resourceNameMatches(%q, %q) = %v, want %v", test.name, test.clusterName, got, test.want)
		}
	}
}

func TestHealthcheckPureHelpers(t *testing.T) {
	if role := parseKeeperRole("zk_version\t1\nzk_server_state\tleader\n"); role != "leader" {
		t.Fatalf("expected leader, got %q", role)
	}
	rows := parseTSV("1\t2\thost-a\n2\t1\thost-b\n")
	if len(rows) != 2 || rows[1][2] != "host-b" {
		t.Fatalf("unexpected TSV rows: %#v", rows)
	}
	localDefinition := "CREATE TABLE upm_healthcheck.local_events (`id` UInt64, `shard_key` UInt64, `message` String) ENGINE = ReplicatedMergeTree('/clickhouse/tables/{cluster}/{shard}/upm_healthcheck/local_events', '{replica}') ORDER BY id"
	if !validHealthcheckLocalDefinition(localDefinition) {
		t.Fatalf("expected local table definition to pass")
	}
	if validHealthcheckLocalDefinition(strings.Replace(localDefinition, "shard_key", "wrong_key", 1)) {
		t.Fatalf("expected local table drift to fail")
	}
	distributedDefinition := "CREATE TABLE upm_healthcheck.dist_events AS upm_healthcheck.local_events ENGINE = Distributed(upm_cluster, upm_healthcheck, local_events, shard_key)"
	if !validHealthcheckDistributedDefinition(distributedDefinition) {
		t.Fatalf("expected distributed table definition to pass")
	}
}

func TestHealthcheckWriteSQLAndAssertionsAreTopologyAware(t *testing.T) {
	topology := model.Topology{Shards: 4, ReplicasPerShard: 3, KeeperReplicas: 3}
	rows := healthcheckProbeRowCount(topology)
	if rows != 16 {
		t.Fatalf("expected 16 probe rows, got %d", rows)
	}
	createSQL := healthcheckCreateTablesSQL()
	if strings.Contains(createSQL, "TRUNCATE") || strings.Contains(createSQL, "INSERT") {
		t.Fatalf("table creation SQL must not mutate existing healthcheck data: %s", createSQL)
	}
	writeSQL := healthcheckWriteSQL(rows)
	if !strings.Contains(writeSQL, "TRUNCATE TABLE") || strings.Count(writeSQL, "healthcheck-") != rows {
		t.Fatalf("unexpected write SQL: %s", writeSQL)
	}
	assertions := strings.Join(healthcheckWriteAssertions(topology, rows), "\n")
	if strings.Contains(assertions, "local_rows != 4") || !strings.Contains(assertions, "replicas != 3") || !strings.Contains(assertions, "count() != 4") {
		t.Fatalf("assertions are not topology-aware: %s", assertions)
	}
}

func TestRedactHealthcheckReportRemovesActualSecretValues(t *testing.T) {
	report := model.NewHealthcheckReport("upm-clickhouse", "clickhouse-demo")
	report.AddCheck("failed_query", model.HealthStatusFail, model.HealthSeverityCritical, "query failed with actual-secret-value", map[string]any{
		"stderr": "authentication failed for actual-secret-value",
		"nested": map[string]any{"value": "actual-secret-value"},
	}, time.Now())

	redactHealthcheckReport(&report, []string{"actual-secret-value"})

	payload := report.Checks[0].Message + report.Checks[0].Evidence["stderr"].(string) + report.Checks[0].Evidence["nested"].(map[string]any)["value"].(string)
	if strings.Contains(payload, "actual-secret-value") || !strings.Contains(payload, "[REDACTED]") {
		t.Fatalf("secret value was not redacted: %s", payload)
	}
}

func TestCheckServicesEndpointsRequiresClickHousePorts(t *testing.T) {
	requiredPorts := []corev1.ServicePort{
		{Name: "tcp", Port: 9000},
		{Name: "http", Port: 8123},
		{Name: "interserver", Port: 9009},
		{Name: "metrics", Port: 9363},
	}
	endpointPorts := []corev1.EndpointPort{
		{Name: "tcp", Port: 9000},
		{Name: "http", Port: 8123},
		{Name: "interserver", Port: 9009},
		{Name: "metrics", Port: 9363},
	}
	client := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "clickhouse-demo-0-svc", Namespace: "upm-clickhouse"},
			Spec:       corev1.ServiceSpec{Ports: requiredPorts},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "clickhouse-demo-0-svc", Namespace: "upm-clickhouse"},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
				Ports:     endpointPorts,
			}},
		},
	)
	store := &Store{core: client}
	report := model.NewHealthcheckReport("upm-clickhouse", "clickhouse-demo")
	store.checkServicesEndpoints(context.Background(), &report, "upm-clickhouse", "clickhouse-demo")
	if report.Checks[0].Status != model.HealthStatusPass {
		t.Fatalf("expected service ports check to pass: %#v", report.Checks[0])
	}

	service, err := client.CoreV1().Services("upm-clickhouse").Get(context.Background(), "clickhouse-demo-0-svc", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	service.Spec.Ports = service.Spec.Ports[:3]
	if _, err := client.CoreV1().Services("upm-clickhouse").Update(context.Background(), service, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update service: %v", err)
	}
	report = model.NewHealthcheckReport("upm-clickhouse", "clickhouse-demo")
	store.checkServicesEndpoints(context.Background(), &report, "upm-clickhouse", "clickhouse-demo")
	if report.Checks[0].Status != model.HealthStatusFail {
		t.Fatalf("expected missing metrics service port to fail: %#v", report.Checks[0])
	}
}

func TestMetricSamplesUsesMetricNameAndRemovesInternalLabel(t *testing.T) {
	samples := metricSamples([]promclient.Sample{{
		Metric: map[string]string{
			"__name__":  "ClickHouseProfileEvents_Query",
			"namespace": "upm-clickhouse",
			"pod":       "clickhouse-demo-0",
			"instance":  "10.0.0.1:9363",
		},
		Value: 42,
	}}, "")

	if len(samples) != 1 || samples[0].Name != "ClickHouseProfileEvents_Query" || samples[0].Value != 42 {
		t.Fatalf("unexpected samples: %#v", samples)
	}
	if _, exists := samples[0].Labels["__name__"]; exists {
		t.Fatalf("internal Prometheus metric label must not be returned: %#v", samples[0].Labels)
	}
	if _, exists := samples[0].Labels["instance"]; exists {
		t.Fatalf("unstable infrastructure labels must not be returned: %#v", samples[0].Labels)
	}
}

func TestFirstAvailableMetricsUsesFallbackOnlyWhenNeeded(t *testing.T) {
	querier := &fakePrometheusQuerier{
		results: map[string][]promclient.Sample{
			"fallback": {
				{Metric: map[string]string{"pod": "clickhouse-demo-0"}, Value: 10},
			},
		},
		errors: map[string]error{"primary": errors.New("primary unavailable")},
	}
	samples, found, err := firstAvailableMetrics(context.Background(), querier, "primary", "fallback", "unused")
	if err != nil {
		t.Fatalf("expected fallback query to pass: %v", err)
	}
	if !found || len(samples) != 1 || len(querier.queries) != 2 || querier.queries[1] != "fallback" {
		t.Fatalf("unexpected fallback behavior: found=%v samples=%#v queries=%#v", found, samples, querier.queries)
	}
}

func TestFirstAvailableMetricsDistinguishesAllEmptyFromAllErrors(t *testing.T) {
	emptyQuerier := &fakePrometheusQuerier{results: map[string][]promclient.Sample{}, errors: map[string]error{}}
	samples, found, err := firstAvailableMetrics(context.Background(), emptyQuerier, "primary", "fallback")
	if err != nil || found || len(samples) != 0 {
		t.Fatalf("expected empty successful queries to be reported without error: found=%v samples=%#v err=%v", found, samples, err)
	}

	errorQuerier := &fakePrometheusQuerier{
		results: map[string][]promclient.Sample{},
		errors:  map[string]error{"primary": errors.New("primary failed"), "fallback": errors.New("fallback failed")},
	}
	_, found, err = firstAvailableMetrics(context.Background(), errorQuerier, "primary", "fallback")
	if err == nil || found {
		t.Fatalf("expected all query errors to return an error: found=%v err=%v", found, err)
	}
}

func TestGetMetricsSummaryOrchestration(t *testing.T) {
	namespace := "upm-clickhouse"
	name := "clickhouse-demo"
	queries := metricsSummaryTestQueries(namespace, name)

	tests := []struct {
		name              string
		includePodMonitor bool
		mutatePrometheus  func(*fakePrometheusQuerier)
		wantStatus        string
		wantWarning       string
		wantStorage       int
	}{
		{
			name:              "ready when targets and all metric categories exist",
			includePodMonitor: true,
			wantStatus:        "READY",
			wantStorage:       3,
		},
		{
			name:              "degraded when an expected target is missing",
			includePodMonitor: true,
			mutatePrometheus: func(prom *fakePrometheusQuerier) {
				prom.results[queries.target] = prom.results[queries.target][:3]
			},
			wantStatus:  "DEGRADED",
			wantWarning: "Prometheus target for clickhouse-demo-3 is missing",
			wantStorage: 3,
		},
		{
			name:              "degraded when CPU query fails and other categories remain",
			includePodMonitor: true,
			mutatePrometheus: func(prom *fakePrometheusQuerier) {
				prom.errors[queries.cpu] = errors.New("cpu unavailable")
			},
			wantStatus:  "DEGRADED",
			wantWarning: "cpu metrics query failed",
			wantStorage: 3,
		},
		{
			name:              "degraded when PodMonitor is missing",
			includePodMonitor: false,
			wantStatus:        "DEGRADED",
			wantWarning:       "PodMonitor upm-clickhouse/clickhouse-demo-exporter-podmon is not readable or does not exist",
			wantStorage:       3,
		},
		{
			name:              "storage fallback is used when primary query is empty",
			includePodMonitor: true,
			wantStatus:        "READY",
			wantStorage:       3,
		},
		{
			name:              "degraded when every storage query is empty",
			includePodMonitor: true,
			mutatePrometheus: func(prom *fakePrometheusQuerier) {
				prom.results[queries.storageFallback] = nil
			},
			wantStatus:  "DEGRADED",
			wantWarning: "storage metrics are missing",
			wantStorage: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prometheus := metricsSummaryPrometheusFixtures(namespace, name)
			if test.mutatePrometheus != nil {
				test.mutatePrometheus(prometheus)
			}
			store := metricsSummaryStoreFixture(namespace, name, test.includePodMonitor, prometheus)

			summary, err := store.GetMetricsSummary(context.Background(), namespace, name)
			if err != nil {
				t.Fatalf("GetMetricsSummary returned error: %v", err)
			}
			if summary.Status != test.wantStatus {
				t.Fatalf("status=%s, want %s, warnings=%v", summary.Status, test.wantStatus, summary.Warnings)
			}
			if test.wantWarning != "" && !containsString(summary.Warnings, test.wantWarning) {
				t.Fatalf("warnings=%v, want %q", summary.Warnings, test.wantWarning)
			}
			if len(summary.Targets) != 4 {
				t.Fatalf("expected 4 targets, got %#v", summary.Targets)
			}
			if len(summary.Summary.Storage) != test.wantStorage {
				t.Fatalf("storage sample count=%d, want %d", len(summary.Summary.Storage), test.wantStorage)
			}
			if len(summary.Summary.Memory) == 0 || len(summary.Summary.ClickHouse) == 0 {
				t.Fatalf("expected memory and ClickHouse samples: %#v", summary.Summary)
			}
		})
	}
}

type metricsSummaryQueries struct {
	target          string
	cpu             string
	memory          string
	clickhouse      string
	storagePrimary  string
	storageFallback string
}

func metricsSummaryTestQueries(namespace, name string) metricsSummaryQueries {
	podPattern := name + "-[0-9]+"
	return metricsSummaryQueries{
		target:          fmt.Sprintf(`up{namespace=%q,pod=~%q}`, namespace, podPattern),
		cpu:             fmt.Sprintf(`sum by (pod) (rate(container_cpu_usage_seconds_total{namespace=%q,pod=~%q,container=%q}[2m]))`, namespace, podPattern, clickHouseContainerName),
		memory:          fmt.Sprintf(`container_memory_working_set_bytes{namespace=%q,pod=~%q,container=%q}`, namespace, podPattern, clickHouseContainerName),
		clickhouse:      fmt.Sprintf(`{__name__=~"ClickHouseProfileEvents_(Query|InsertQuery|InsertedRows|InsertedBytes)|ClickHouseMetrics_MemoryTracking",namespace=%q,pod=~%q}`, namespace, podPattern),
		storagePrimary:  fmt.Sprintf(`kubelet_volume_stats_used_bytes{namespace=%q,persistentvolumeclaim=~%q}`, namespace, name+"-[0-9]+-data"),
		storageFallback: fmt.Sprintf(`{__name__=~"ClickHouseAsyncMetrics_Disk(Used|Total|Available)_default",namespace=%q,pod=~%q}`, namespace, podPattern),
	}
}

func metricsSummaryPrometheusFixtures(namespace, name string) *fakePrometheusQuerier {
	queries := metricsSummaryTestQueries(namespace, name)
	pods := []string{name + "-0", name + "-1", name + "-2", name + "-3"}
	targets := make([]promclient.Sample, 0, len(pods))
	cpu := make([]promclient.Sample, 0, len(pods))
	memory := make([]promclient.Sample, 0, len(pods))
	clickhouse := make([]promclient.Sample, 0, len(pods))
	storage := []promclient.Sample{}
	for index, pod := range pods {
		targets = append(targets, promclient.Sample{Metric: map[string]string{"namespace": namespace, "pod": pod}, Value: 1})
		cpu = append(cpu, promclient.Sample{Metric: map[string]string{"namespace": namespace, "pod": pod}, Value: float64(index + 1)})
		memory = append(memory, promclient.Sample{Metric: map[string]string{"namespace": namespace, "pod": pod}, Value: float64(1024 * (index + 1))})
		clickhouse = append(clickhouse, promclient.Sample{Metric: map[string]string{"__name__": "ClickHouseProfileEvents_Query", "namespace": namespace, "pod": pod}, Value: float64(10 + index)})
	}
	for _, metric := range []string{
		"ClickHouseAsyncMetrics_DiskUsed_default",
		"ClickHouseAsyncMetrics_DiskTotal_default",
		"ClickHouseAsyncMetrics_DiskAvailable_default",
	} {
		storage = append(storage, promclient.Sample{Metric: map[string]string{"__name__": metric, "namespace": namespace, "pod": pods[0]}, Value: 100})
	}
	return &fakePrometheusQuerier{
		results: map[string][]promclient.Sample{
			queries.target:          targets,
			queries.cpu:             cpu,
			queries.memory:          memory,
			queries.clickhouse:      clickhouse,
			queries.storagePrimary:  {},
			queries.storageFallback: storage,
		},
		errors: map[string]error{},
	}
}

func metricsSummaryStoreFixture(namespace, name string, includePodMonitor bool, prometheus *fakePrometheusQuerier) *Store {
	request := model.CreateClusterRequest{
		Namespace: namespace,
		Name:      name,
		Version:   "26.3.9.8",
		Topology:  model.Topology{Shards: 2, ReplicasPerShard: 2, KeeperReplicas: 3},
		Storage: model.Storage{
			ClassName:      "local-path",
			ServerDataSize: "20Gi",
			KeeperDataSize: "10Gi",
		},
		Security:   model.Security{AdminSecretRef: name + "-secret"},
		Monitoring: model.MonitoringConfig{Enabled: true},
	}
	dynamicObjects := []runtime.Object{serverUnitSet(request), keeperUnitSet(request)}
	if includePodMonitor {
		dynamicObjects = append(dynamicObjects, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "monitoring.coreos.com/v1",
			"kind":       "PodMonitor",
			"metadata": map[string]any{
				"name":      name + "-exporter-podmon",
				"namespace": namespace,
			},
		}})
	}
	coreObjects := []runtime.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "clickhouse-26.3.9.8-config-value", Namespace: managerNamespace},
			Data:       map[string]string{"clickhouse": "topology:\n  shards: \"2\"\n  replicasPerShard: \"2\"\nkeeper:\n  replicas: \"3\"\n"},
		},
	}
	for index := 0; index < 4; index++ {
		coreObjects = append(coreObjects, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", name, index),
				Namespace: namespace,
				Labels:    map[string]string{serviceGroupNameLabel: name, serviceTypeLabel: "clickhouse"},
			},
		})
	}
	return NewStoreWithPrometheus(
		dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), dynamicObjects...),
		fake.NewSimpleClientset(coreObjects...),
		prometheus,
	)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
