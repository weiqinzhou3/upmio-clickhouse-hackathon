package config

import (
	"os"
	"strconv"
	"time"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/model"
)

type Config struct {
	ListenAddr            string
	RequestTimeout        time.Duration
	PrometheusBaseURL     string
	PrometheusTimeout     time.Duration
	DiagnosticsThresholds model.DiagnosticsThresholds
}

func FromEnv() Config {
	return Config{
		ListenAddr:        envOrDefault("LISTEN_ADDR", ":8080"),
		RequestTimeout:    durationOrDefault("REQUEST_TIMEOUT", 10*time.Minute),
		PrometheusBaseURL: envOrDefault("PROMETHEUS_BASE_URL", "http://kube-prometheus-stack-prometheus.monitoring.svc:9090"),
		PrometheusTimeout: durationOrDefault("PROMETHEUS_TIMEOUT", 10*time.Second),
		DiagnosticsThresholds: model.DiagnosticsThresholds{
			ReplicaDelayWarnSeconds:     intOrDefault("DIAGNOSTICS_REPLICA_DELAY_WARN_SECONDS", 60),
			ReplicaDelayCriticalSeconds: intOrDefault("DIAGNOSTICS_REPLICA_DELAY_CRITICAL_SECONDS", 300),
			ReplicationQueueWarn:        intOrDefault("DIAGNOSTICS_REPLICATION_QUEUE_WARN", 50),
			ReplicationQueueCritical:    intOrDefault("DIAGNOSTICS_REPLICATION_QUEUE_CRITICAL", 500),
			ActivePartsPerPartitionWarn: intOrDefault("DIAGNOSTICS_ACTIVE_PARTS_PER_PARTITION_WARN", 150),
			InactivePartsWarn:           intOrDefault("DIAGNOSTICS_INACTIVE_PARTS_WARN", 50),
			MergeElapsedWarnSeconds:     intOrDefault("DIAGNOSTICS_MERGE_ELAPSED_WARN_SECONDS", 1800),
			MutationAgeWarnSeconds:      intOrDefault("DIAGNOSTICS_MUTATION_AGE_WARN_SECONDS", 600),
		}.WithDefaults(),
	}
}

func envOrDefault(name, defaultValue string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return defaultValue
}

func durationOrDefault(name string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return duration
}

func intOrDefault(name string, defaultValue int) int {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}
