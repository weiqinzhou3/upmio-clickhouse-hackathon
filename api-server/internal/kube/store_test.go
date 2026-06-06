package kube

import (
	"context"
	"testing"

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
