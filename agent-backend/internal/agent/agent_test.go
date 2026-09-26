package agent

import (
	"context"
	"iter"
	"strings"
	"sync/atomic"
	"testing"

	"ai-ops-copilot/agent-backend/internal/memory"
	"ai-ops-copilot/agent-backend/internal/tools"
	adkmodel "google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/genai"
)

type recordingExecutor struct {
	name   string
	args   map[string]any
	result map[string]any
}

func (e *recordingExecutor) ADKTools() ([]tool.Tool, error) { return nil, nil }
func (e *recordingExecutor) Execute(_ context.Context, name string, args map[string]any) (map[string]any, error) {
	e.name, e.args = name, args
	return e.result, nil
}

type scriptedADKModel struct{ calls atomic.Int32 }

func (m *scriptedADKModel) Name() string { return "scripted-adk-model" }

func (m *scriptedADKModel) GenerateContent(_ context.Context, _ *adkmodel.LLMRequest, _ bool) iter.Seq2[*adkmodel.LLMResponse, error] {
	call := m.calls.Add(1)
	return func(yield func(*adkmodel.LLMResponse, error) bool) {
		if call == 1 {
			content := genai.NewContentFromFunctionCall(tools.ToolGetFirestoreDigest, map[string]any{"lookback_hours": 2}, genai.RoleModel)
			yield(&adkmodel.LLMResponse{Content: content, TurnComplete: true}, nil)
			return
		}
		yield(&adkmodel.LLMResponse{
			Content: genai.NewContentFromText("Consultei a fonte do Grafana.", genai.RoleModel), TurnComplete: true,
		}, nil)
	}
}

func TestFormatScreenContext(t *testing.T) {
	if got := FormatScreenContext(nil); got != "" {
		t.Fatalf("expected empty string for nil context, got %q", got)
	}
	ctx := &DashboardContext{
		DashboardUID: "aiops-telemetry-prod", DashboardTitle: "Telemetry Ingestion Overview",
		ActivePanel: "12", ActivePanelTitle: "Percentile Latencies",
	}
	formatted := FormatScreenContext(ctx)
	for _, want := range []string{"aiops-telemetry-prod", "Percentile Latencies", "\"active_panel\": \"12\""} {
		if !strings.Contains(formatted, want) {
			t.Errorf("expected screen context to contain %q, got %s", want, formatted)
		}
	}
}

func TestOrchestratorMockChatUsesActivePanel(t *testing.T) {
	ctx := context.Background()
	executor := tools.NewExecutor(
		tools.NewMockGrafanaClient(), tools.NewMockMonitoringClient(),
		tools.NewMockLoggingClient(), tools.NewMockDigestClient(), nil,
	)
	store := memory.NewInMemoryStore()
	defer store.Close()
	orchestrator := NewOrchestrator(nil, "gemini-3.8-flash", executor, store, true, nil)

	if _, err := orchestrator.Chat(ctx, ChatRequest{Message: "  "}); err == nil {
		t.Fatal("expected an error for an empty message")
	}
	response, err := orchestrator.Chat(ctx, ChatRequest{
		UserID: "operator", SessionID: "session-1", Message: "Como estão as métricas desse painel?",
		Context: &DashboardContext{
			DashboardUID: "aiops-telemetry-prod", ActivePanel: "12",
			TimeRange: map[string]any{"from": "now-6h", "to": "now"},
		},
	})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Name != tools.ToolGetDashboardMetrics {
		t.Fatalf("expected one read-only Grafana panel query, got %+v", response.ToolCalls)
	}
	if response.ToolCalls[0].Result["status"] != "mock" {
		t.Fatalf("mock response was not marked as simulated: %+v", response.ToolCalls[0].Result)
	}
	if !strings.Contains(response.Response, "Modo simulado") {
		t.Fatalf("mock response should disclose simulation: %s", response.Response)
	}
	history, err := store.GetHistory(ctx, "session-1", 20)
	if err != nil {
		t.Fatalf("read conversation history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected user and assistant messages, got %d", len(history))
	}
}

func TestOrchestratorRunsADKToolCalls(t *testing.T) {
	store := memory.NewInMemoryStore()
	defer store.Close()
	executor := tools.NewExecutor(
		tools.NewMockGrafanaClient(), tools.NewMockMonitoringClient(),
		tools.NewMockLoggingClient(), tools.NewMockDigestClient(), nil,
	)
	model := &scriptedADKModel{}
	orchestrator := NewOrchestrator(model, "gemini-3.8-flash", executor, store, false, nil)

	response, err := orchestrator.Chat(context.Background(), ChatRequest{
		UserID: "operator", SessionID: "adk-session", Message: "Resuma a saúde da infraestrutura.",
	})
	if err != nil {
		t.Fatalf("ADK chat failed: %v", err)
	}
	if model.calls.Load() != 2 {
		t.Fatalf("expected one tool round and one synthesis round, got %d model calls", model.calls.Load())
	}
	if response.Response != "Consultei a fonte do Grafana." {
		t.Fatalf("unexpected ADK final response: %q", response.Response)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Name != tools.ToolGetFirestoreDigest {
		t.Fatalf("expected the ADK read-only digest tool to be invoked, got %+v", response.ToolCalls)
	}
	if response.ToolCalls[0].Result["lookback_hours"] != float64(2) {
		t.Fatalf("expected the ADK result from the tool executor, got %+v", response.ToolCalls[0].Result)
	}
}

func TestMockChatRoutesBillingWithoutInventingCosts(t *testing.T) {
	executor := &recordingExecutor{result: map[string]any{"error": "Cloud Billing export unavailable"}}
	store := memory.NewInMemoryStore()
	defer store.Close()
	orchestrator := NewOrchestrator(nil, "", executor, store, true, nil)

	response, err := orchestrator.Chat(context.Background(), ChatRequest{
		SessionID: "billing", Message: "Qual o custo total de todos os projetos?",
	})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if executor.name != tools.ToolQueryBillingCosts {
		t.Fatalf("expected billing tool, got %q", executor.name)
	}
	if !strings.Contains(response.Response, "Modo simulado") || !strings.Contains(response.Response, "unavailable") {
		t.Fatalf("expected an explicit mock and source error, got %s", response.Response)
	}
	if strings.Contains(response.Response, "R$ 0") || strings.Contains(response.Response, "US$ 0") {
		t.Fatalf("response must not invent a zero cost: %s", response.Response)
	}
}

func TestMockChatFollowsMessageLanguage(t *testing.T) {
	for _, test := range []struct {
		message string
		want    string
	}{
		{message: "Como está o projeto?", want: "Modo simulado"},
		{message: "How is the project?", want: "Local development mock mode"},
	} {
		t.Run(test.message, func(t *testing.T) {
			store := memory.NewInMemoryStore()
			defer store.Close()
			orchestrator := NewOrchestrator(nil, "", &recordingExecutor{
				result: map[string]any{"summary": "Fonte simulada"},
			}, store, true, nil)
			response, err := orchestrator.Chat(context.Background(), ChatRequest{Message: test.message})
			if err != nil {
				t.Fatalf("chat failed: %v", err)
			}
			if !strings.Contains(response.Response, test.want) {
				t.Fatalf("expected language marker %q, got %s", test.want, response.Response)
			}
		})
	}
}

func TestOrchestratorGenerateDigestDoesNotInferHealth(t *testing.T) {
	store := memory.NewInMemoryStore()
	defer store.Close()
	orchestrator := NewOrchestrator(nil, "", &recordingExecutor{result: map[string]any{}}, store, true, nil)

	snapshot, err := orchestrator.GenerateDigest(context.Background(), 2)
	if err != nil {
		t.Fatalf("generate digest: %v", err)
	}
	if strings.Contains(strings.ToLower(snapshot.Summary), "operacionais sem anomalias") {
		t.Fatalf("digest fallback claimed unverified health: %s", snapshot.Summary)
	}
}
