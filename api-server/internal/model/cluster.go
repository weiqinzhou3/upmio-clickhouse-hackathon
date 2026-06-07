package model

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
)

type CreateClusterRequest struct {
	Namespace  string           `json:"namespace"`
	Name       string           `json:"name"`
	Version    string           `json:"version"`
	Topology   Topology         `json:"topology"`
	Storage    Storage          `json:"storage"`
	Security   Security         `json:"security"`
	Monitoring MonitoringConfig `json:"monitoring"`
}

type Topology struct {
	Shards           int `json:"shards"`
	ReplicasPerShard int `json:"replicasPerShard"`
	KeeperReplicas   int `json:"keeperReplicas"`
}

type Storage struct {
	ClassName      string `json:"className"`
	ServerDataSize string `json:"serverDataSize"`
	KeeperDataSize string `json:"keeperDataSize"`
}

type Security struct {
	AdminSecretRef string `json:"adminSecretRef"`
}

type MonitoringConfig struct {
	Enabled bool `json:"enabled"`
}

type ClusterSummary struct {
	Namespace string             `json:"namespace"`
	Name      string             `json:"name"`
	Version   string             `json:"version,omitempty"`
	Status    string             `json:"status"`
	Topology  Topology           `json:"topology"`
	Ready     ReadySummary       `json:"ready"`
	Resources ClusterResourceRef `json:"resources,omitempty"`
	RequestID string             `json:"requestId,omitempty"`
}

type ClusterList struct {
	Items []ClusterSummary `json:"items"`
}

type ReadySummary struct {
	Keeper string `json:"keeper"`
	Server string `json:"server"`
}

type ClusterResourceRef struct {
	Project       string `json:"project,omitempty"`
	KeeperUnitSet string `json:"keeperUnitSet,omitempty"`
	ServerUnitSet string `json:"serverUnitSet,omitempty"`
}

type ResourceSummary struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace,omitempty"`
	Kind      string            `json:"kind"`
	Status    string            `json:"status,omitempty"`
	Details   map[string]any    `json:"details,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type ClusterResources struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	UnitSets  []ResourceSummary `json:"unitSets"`
	Units     []ResourceSummary `json:"units"`
	Pods      []ResourceSummary `json:"pods"`
	PVCs      []ResourceSummary `json:"pvcs"`
	Services  []ResourceSummary `json:"services"`
	Endpoints []ResourceSummary `json:"endpoints"`
	Events    []ResourceSummary `json:"events"`
}

func (r CreateClusterRequest) Validate() error {
	if err := ValidateClusterIdentity(r.Namespace, r.Name); err != nil {
		return err
	}
	if r.Version == "" {
		return fmt.Errorf("version is required")
	}
	if r.Topology.Shards < 1 {
		return fmt.Errorf("topology.shards must be greater than 0")
	}
	if r.Topology.ReplicasPerShard < 1 {
		return fmt.Errorf("topology.replicasPerShard must be greater than 0")
	}
	if r.Topology.KeeperReplicas < 1 {
		return fmt.Errorf("topology.keeperReplicas must be greater than 0")
	}
	if r.Storage.ClassName == "" {
		return fmt.Errorf("storage.className is required")
	}
	if _, err := resource.ParseQuantity(r.Storage.ServerDataSize); err != nil {
		return fmt.Errorf("storage.serverDataSize: %w", err)
	}
	if _, err := resource.ParseQuantity(r.Storage.KeeperDataSize); err != nil {
		return fmt.Errorf("storage.keeperDataSize: %w", err)
	}
	if errs := k8svalidation.IsDNS1123Subdomain(r.Security.AdminSecretRef); len(errs) > 0 {
		return fmt.Errorf("security.adminSecretRef: %s", errs[0])
	}
	return nil
}

func ValidateClusterIdentity(namespace, name string) error {
	if err := ValidateDNSLabel("namespace", namespace); err != nil {
		return err
	}
	if errs := k8svalidation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("name: %s", errs[0])
	}
	return nil
}

func ValidateNamespace(namespace string) error {
	return ValidateDNSLabel("namespace", namespace)
}

func ValidateDNSLabel(field, value string) error {
	if errs := k8svalidation.IsDNS1123Label(value); len(errs) > 0 {
		return fmt.Errorf("%s: %s", field, errs[0])
	}
	return nil
}
