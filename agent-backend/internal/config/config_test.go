package config

import (
	"os"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	for _, name := range []string{
		"PORT", "GCP_PROJECT_ID", "VERTEX_PROJECT_ID", "GCP_LOCATION", "GEMINI_MODEL",
		"BILLING_PROJECT_ID", "BILLING_DATASET", "GRAFANA_URL", "GRAFANA_LOCAL_URL", "LOCAL_MOCK_MODE",
	} {
		t.Setenv(name, "")
	}

	cfg := Load()
	if cfg.Port != "8080" || cfg.GCPProjectID != "enterprise-core-prod" || cfg.VertexProjectID != "enterprise-core-prod" {
		t.Fatalf("unexpected project or port defaults: %+v", cfg)
	}
	if cfg.GCPLocation != "us-central1" || cfg.GeminiModel != "gemini-3.8-flash" {
		t.Fatalf("unexpected Vertex AI defaults: %+v", cfg)
	}
	if cfg.GrafanaURL != "http://localhost:3000" || cfg.LocalMockMode {
		t.Fatalf("unexpected Grafana or mock-mode defaults: %+v", cfg)
	}
	if cfg.BillingProjectID != "" || cfg.BillingDataset != "" {
		t.Fatalf("billing source must be injected by runtime configuration, got %+v", cfg)
	}
}

func TestConfigCustomEnvironment(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("GCP_PROJECT_ID", "enterprise-telemetry-prod")
	t.Setenv("VERTEX_PROJECT_ID", "enterprise-ai-gateway")
	t.Setenv("GCP_LOCATION", "us-central1")
	t.Setenv("GEMINI_MODEL", "gemini-3.8-flash")
	t.Setenv("BILLING_PROJECT_ID", "enterprise-ai-analytics")
	t.Setenv("BILLING_DATASET", "billing_export")
	t.Setenv("GRAFANA_LOCAL_URL", "http://grafana-internal:3000")
	t.Setenv("LOCAL_MOCK_MODE", "true")

	cfg := Load()
	if cfg.Port != "9090" || cfg.GCPProjectID != "enterprise-telemetry-prod" || cfg.VertexProjectID != "enterprise-ai-gateway" {
		t.Fatalf("unexpected custom project or port: %+v", cfg)
	}
	if cfg.BillingProjectID != "enterprise-ai-analytics" || cfg.BillingDataset != "billing_export" {
		t.Fatalf("unexpected billing config: %+v", cfg)
	}
	if cfg.GrafanaURL != "http://grafana-internal:3000" || !cfg.LocalMockMode {
		t.Fatalf("unexpected custom Grafana or mock mode: %+v", cfg)
	}
}

func TestConfigVertexProjectFallback(t *testing.T) {
	t.Setenv("GCP_PROJECT_ID", "enterprise-saas-staging")
	os.Unsetenv("VERTEX_PROJECT_ID")
	t.Cleanup(func() { os.Unsetenv("GCP_PROJECT_ID") })
	if cfg := Load(); cfg.VertexProjectID != "enterprise-saas-staging" {
		t.Fatalf("expected Vertex project fallback enterprise-saas-staging, got %q", cfg.VertexProjectID)
	}
}
