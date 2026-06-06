package model

import "time"

type MetricsSummary struct {
	RequestID   string                 `json:"requestId,omitempty"`
	Namespace   string                 `json:"namespace"`
	Name        string                 `json:"name"`
	Cluster     string                 `json:"cluster"`
	Status      string                 `json:"status"`
	CollectedAt time.Time              `json:"collectedAt"`
	PodMonitor  MetricsPodMonitor      `json:"podMonitor"`
	Targets     []MetricsTarget        `json:"targets"`
	Summary     MetricsCategorySummary `json:"summary"`
	Warnings    []string               `json:"warnings"`
}

type MetricsPodMonitor struct {
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
}

type MetricsTarget struct {
	Name string `json:"name"`
	Up   bool   `json:"up"`
}

type MetricsCategorySummary struct {
	CPU        []MetricSample `json:"cpu"`
	Memory     []MetricSample `json:"memory"`
	Storage    []MetricSample `json:"storage"`
	ClickHouse []MetricSample `json:"clickhouse"`
}

type MetricSample struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
	Unit   string            `json:"unit,omitempty"`
}
