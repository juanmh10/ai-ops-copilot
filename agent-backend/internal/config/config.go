package config

import (
	"os"
	"strconv"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	Port                  string
	GCPProjectID          string
	VertexProjectID       string
	GCPLocation           string
	GeminiModel           string
	BillingProjectID      string
	BillingDataset        string
	GrafanaURL            string
	FirestoreEmulatorHost string
	LocalMockMode         bool
	AllowedOrigins        string
}

// Load loads configuration from environment variables with safe defaults.
func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" {
		projectID = "enterprise-core-prod"
	}

	vertexProjectID := os.Getenv("VERTEX_PROJECT_ID")
	if vertexProjectID == "" {
		vertexProjectID = projectID
	}

	location := os.Getenv("GCP_LOCATION")
	if location == "" {
		location = "us-central1"
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-3.8-flash"
	}

	billingProjectID := os.Getenv("BILLING_PROJECT_ID")
	billingDataset := os.Getenv("BILLING_DATASET")

	grafanaURL := os.Getenv("GRAFANA_URL")
	if grafanaURL == "" {
		grafanaURL = os.Getenv("GRAFANA_LOCAL_URL")
	}
	if grafanaURL == "" {
		grafanaURL = "http://localhost:3000"
	}

	mockModeStr := os.Getenv("LOCAL_MOCK_MODE")
	mockMode := false
	if b, err := strconv.ParseBool(mockModeStr); err == nil {
		mockMode = b
	}

	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = "*"
	}

	return &Config{
		Port:                  port,
		GCPProjectID:          projectID,
		VertexProjectID:       vertexProjectID,
		GCPLocation:           location,
		GeminiModel:           model,
		BillingProjectID:      billingProjectID,
		BillingDataset:        billingDataset,
		GrafanaURL:            grafanaURL,
		FirestoreEmulatorHost: os.Getenv("FIRESTORE_EMULATOR_HOST"),
		LocalMockMode:         mockMode,
		AllowedOrigins:        allowedOrigins,
	}
}
