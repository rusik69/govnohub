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
	AIReview       AIReviewConfig
	BootstrapAdminUsername  string
	BootstrapAdminEmail     string
	BootstrapAdminPassword  string
	AllowPublicRegistration bool
}

type AIReviewConfig struct {
	Enabled bool
	APIKey  string
	BaseURL string
	Model   string
	Auto    bool
}

func Load() Config {
	apiKey := getEnv("OPENCODE_API_KEY", getEnv("AI_REVIEW_API_KEY", ""))
	enabled := getEnvBool("AI_REVIEW_ENABLED", apiKey != "")
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
		AIReview: AIReviewConfig{
			Enabled: enabled,
			APIKey:  apiKey,
			BaseURL: getEnv("AI_REVIEW_BASE_URL", "https://opencode.ai/zen/go/v1"),
			Model:   getEnv("AI_REVIEW_MODEL", "deepseek-v4-flash"),
			Auto:    getEnvBool("AI_REVIEW_AUTO", false),
		},
		BootstrapAdminUsername:  getEnv("BOOTSTRAP_ADMIN_USERNAME", "admin"),
		BootstrapAdminEmail:     getEnv("BOOTSTRAP_ADMIN_EMAIL", "admin@govnohub.local"),
		BootstrapAdminPassword:  getEnv("BOOTSTRAP_ADMIN_PASSWORD", "admin"),
		AllowPublicRegistration: getEnvBool("ALLOW_PUBLIC_REGISTRATION", false),
	}
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
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
