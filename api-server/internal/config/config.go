package config

import (
	"os"
	"time"
)

type Config struct {
	ListenAddr        string
	RequestTimeout    time.Duration
	PrometheusBaseURL string
	PrometheusTimeout time.Duration
}

func FromEnv() Config {
	return Config{
		ListenAddr:        envOrDefault("LISTEN_ADDR", ":8080"),
		RequestTimeout:    durationOrDefault("REQUEST_TIMEOUT", 10*time.Minute),
		PrometheusBaseURL: envOrDefault("PROMETHEUS_BASE_URL", "http://kube-prometheus-stack-prometheus.monitoring.svc:9090"),
		PrometheusTimeout: durationOrDefault("PROMETHEUS_TIMEOUT", 10*time.Second),
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
