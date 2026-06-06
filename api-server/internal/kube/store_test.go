package kube

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"
)

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
