package model

import "testing"

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
