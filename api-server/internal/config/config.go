package config

import (
	"os"
	"time"
)

type Config struct {
	ListenAddr     string
	RequestTimeout time.Duration
}

func FromEnv() Config {
	return Config{
		ListenAddr:     envOrDefault("LISTEN_ADDR", ":8080"),
		RequestTimeout: durationOrDefault("REQUEST_TIMEOUT", 10*time.Minute),
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
