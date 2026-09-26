package main

import (
	"context"
	"testing"

	"ai-ops-copilot/agent-backend/internal/config"
	"google.golang.org/genai"
)

func TestBuildGenAIClientConfigUsesVertexADC(t *testing.T) {
	cfg := &config.Config{
		VertexProjectID: "enterprise-ai-gateway",
		GCPLocation:     "us-central1",
	}
	clientConfig := buildGenAIClientConfig(cfg)
	if clientConfig.Backend != genai.BackendVertexAI {
		t.Fatalf("expected Vertex AI backend, got %v", clientConfig.Backend)
	}
	if clientConfig.Project != "enterprise-ai-gateway" || clientConfig.Location != "us-central1" {
		t.Fatalf("unexpected Vertex AI target: project=%q location=%q", clientConfig.Project, clientConfig.Location)
	}
	if clientConfig.APIKey != "" {
		t.Fatal("Vertex AI configuration must use ADC rather than an API key")
	}
}

func TestInitADKModelMockMode(t *testing.T) {
	llm, mock := initADKModel(context.Background(), &config.Config{LocalMockMode: true}, nil)
	if llm != nil || !mock {
		t.Fatalf("expected a nil LLM and explicit mock mode, got llm=%v mock=%v", llm, mock)
	}
}
