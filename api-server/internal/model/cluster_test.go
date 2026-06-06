package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCreateClusterRequestValidate(t *testing.T) {
	valid := CreateClusterRequest{
		Namespace: "upm-clickhouse",
		Name:      "ch-demo",
		Version:   "26.3.9.8",
		Topology: Topology{
			Shards:           2,
			ReplicasPerShard: 2,
			KeeperReplicas:   3,
		},
		Storage: Storage{
			ClassName:      "local-path",
			ServerDataSize: "20Gi",
			KeeperDataSize: "10Gi",
		},
		Security: Security{AdminSecretRef: "ch-demo-secret"},
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*CreateClusterRequest)
	}{
		{"invalid namespace", func(r *CreateClusterRequest) { r.Namespace = "bad namespace" }},
		{"missing version", func(r *CreateClusterRequest) { r.Version = "" }},
		{"zero shards", func(r *CreateClusterRequest) { r.Topology.Shards = 0 }},
		{"zero replicas", func(r *CreateClusterRequest) { r.Topology.ReplicasPerShard = 0 }},
		{"zero keepers", func(r *CreateClusterRequest) { r.Topology.KeeperReplicas = 0 }},
		{"invalid server storage", func(r *CreateClusterRequest) { r.Storage.ServerDataSize = "invalid" }},
		{"missing admin secret", func(r *CreateClusterRequest) { r.Security.AdminSecretRef = "" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			if err := request.Validate(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestResponseModelsDoNotExposeSecurityFields(t *testing.T) {
	responseTypes := []reflect.Type{
		reflect.TypeOf(ClusterSummary{}),
		reflect.TypeOf(ClusterList{}),
		reflect.TypeOf(ReadySummary{}),
		reflect.TypeOf(ClusterResourceRef{}),
		reflect.TypeOf(ResourceSummary{}),
		reflect.TypeOf(ClusterResources{}),
		reflect.TypeOf(ErrorResponse{}),
	}
	for _, responseType := range responseTypes {
		for i := 0; i < responseType.NumField(); i++ {
			field := responseType.Field(i)
			fieldName := strings.ToLower(field.Name)
			jsonTag := strings.ToLower(field.Tag.Get("json"))
			if strings.Contains(fieldName, "secret") ||
				strings.Contains(fieldName, "password") ||
				strings.Contains(jsonTag, "secret") ||
				strings.Contains(jsonTag, "password") {
				t.Fatalf("%s exposes secret-like field %s with json tag %q", responseType.Name(), field.Name, field.Tag.Get("json"))
			}
		}
	}
}

func TestClusterSummaryJSONDoesNotContainSecurityMaterial(t *testing.T) {
	summary := ClusterSummary{
		Namespace: "upm-clickhouse",
		Name:      "ch-demo",
		Version:   "26.3.9.8",
		Status:    "Running",
		Topology:  Topology{Shards: 2, ReplicasPerShard: 2, KeeperReplicas: 3},
		Ready:     ReadySummary{Keeper: "3/3", Server: "4/4"},
		Resources: ClusterResourceRef{
			Project:       "upm-clickhouse",
			KeeperUnitSet: "ch-demo-keeper",
			ServerUnitSet: "ch-demo",
		},
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal ClusterSummary: %v", err)
	}
	lower := strings.ToLower(string(data))
	for _, forbidden := range []string{"security", "secret", "password"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("ClusterSummary JSON must not contain %q: %s", forbidden, data)
		}
	}
}
