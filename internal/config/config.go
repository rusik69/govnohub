package config

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr       string
	DatabaseURL    string
	JWTSecret      string
	GitRoot        string
	ArtifactRoot   string
	RunnerNS       string
	OpenSearchURL  string
	WebhookURL     string
	RegistryURL    string
}

func Load() Config {
	return Config{
		HTTPAddr:      getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://govnohub:govnohub@localhost:5432/govnohub?sslmode=disable"),
		JWTSecret:     getEnv("JWT_SECRET", "dev-secret-change-me"),
		GitRoot:       getEnv("GIT_ROOT", "/data/git"),
		ArtifactRoot:  getEnv("ARTIFACT_ROOT", "/data/artifacts"),
		RunnerNS:      getEnv("RUNNER_NAMESPACE", "govnohub-runners"),
		OpenSearchURL: getEnv("OPENSEARCH_URL", "http://localhost:9200"),
		WebhookURL:    getEnv("WEBHOOK_SERVICE_URL", "http://webhook-service:8082"),
		RegistryURL:   getEnv("REGISTRY_URL", "registry.govnohub.local:5000"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func GetEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
