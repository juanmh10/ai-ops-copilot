package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ai-ops-copilot/agent-backend/internal/memory"
	"ai-ops-copilot/agent-backend/internal/tools"

	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/artifact"
	adkmemory "google.golang.org/adk/v2/memory"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// DashboardContext captures the visual context from Grafana.
type DashboardContext struct {
	DashboardUID     string           `json:"dashboard_uid,omitempty"`
	DashboardTitle   string           `json:"dashboard_title,omitempty"`
	TimeRange        map[string]any   `json:"time_range,omitempty"`
	ActivePanel      string           `json:"active_panel,omitempty"`
	ActivePanelTitle string           `json:"active_panel_title,omitempty"`
	ActiveFilters    map[string]any   `json:"active_filters,omitempty"`
	ScopeLevel       string           `json:"scope_level,omitempty"`
	ProjectID        string           `json:"project_id,omitempty"`
	ProjectIDs       []string         `json:"project_ids,omitempty"`
	Environment      string           `json:"environment,omitempty"`
	ServiceNames     []string         `json:"service_names,omitempty"`
	ResolvedScope    *ScopeResolution `json:"resolved_scope,omitempty"`
}

// ChatRequest represents the payload from the Grafana chat plugin.
type ChatRequest struct {
	UserID    string            `json:"user_id"`
	SessionID string            `json:"session_id"`
	Message   string            `json:"message"`
	Context   *DashboardContext `json:"context,omitempty"`
}

// InvokedTool represents a tool executed during a turn.
type InvokedTool struct {
	Name   string         `json:"name"`
	Args   map[string]any `json:"args"`
	Result map[string]any `json:"result,omitempty"`
}

// ChatResponse represents the assistant's answer and tool execution trace.
type ChatResponse struct {
	Response  string        `json:"response"`
	SessionID string        `json:"session_id"`
	ToolCalls []InvokedTool `json:"tool_calls,omitempty"`
}

// Agent is the conversational interface for the AI-Ops Copilot.
type Agent interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	GenerateDigest(ctx context.Context, lookbackHours int) (*tools.DigestSnapshot, error)
}

// Orchestrator coordinates Gemini Vertex AI, tools, and session memory.
type Orchestrator struct {
	llm           model.LLM
	modelName     string
	toolsExecutor tools.ToolExecutor
	memoryStore   memory.SessionStore
	mockMode      bool
	logger        *slog.Logger
}

// NewOrchestrator creates a new Orchestrator.
func NewOrchestrator(
	llm model.LLM,
	model string,
	toolsExecutor tools.ToolExecutor,
	memoryStore memory.SessionStore,
	mockMode bool,
	logger *slog.Logger,
) *Orchestrator {
	if logger == nil {
		logger = slog.Default()
	}
	if model == "" {
		model = "gemini-3.8-flash"
	}
	return &Orchestrator{
		llm:           llm,
		modelName:     model,
		toolsExecutor: toolsExecutor,
		memoryStore:   memoryStore,
		mockMode:      mockMode,
		logger:        logger,
	}
}

// Chat handles a conversation turn.
func (o *Orchestrator) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	sessionID := req.SessionID
	if sessionID == "" {
		if req.UserID != "" {
			sessionID = req.UserID
		} else {
			sessionID = "default-session"
		}
	}

	if strings.TrimSpace(req.Message) == "" {
		return nil, fmt.Errorf("message cannot be empty")
	}
	if req.Context == nil {
		req.Context = &DashboardContext{}
	}
	toolScope := tools.DashboardQueryContext{
		DashboardUID: req.Context.DashboardUID, ActiveFilters: make(map[string]string),
	}
	if req.Context.TimeRange != nil {
		toolScope.TimeFrom, _ = req.Context.TimeRange["from"].(string)
		toolScope.TimeTo, _ = req.Context.TimeRange["to"].(string)
	}
	for key, value := range req.Context.ActiveFilters {
		if text, ok := value.(string); ok {
			toolScope.ActiveFilters[key] = text
		}
	}
	ctx = tools.MergeDashboardQueryContext(ctx, toolScope)
	scope := ResolveQueryScope(req.Message, req.Context)
	req.Context.ResolvedScope = &scope

	// Prepare user message
	userMsg := memory.ChatMessage{
		ID:        fmt.Sprintf("usr-%d", time.Now().UnixNano()),
		Role:      "user",
		Content:   req.Message,
		Timestamp: time.Now().UTC(),
	}
	if req.Context != nil {
		userMsg.Context = map[string]any{
			"dashboard_uid":      req.Context.DashboardUID,
			"dashboard_title":    req.Context.DashboardTitle,
			"active_panel":       req.Context.ActivePanel,
			"active_panel_title": req.Context.ActivePanelTitle,
			"time_range":         req.Context.TimeRange,
			"active_filters":     req.Context.ActiveFilters,
			"project_id":         req.Context.ProjectID,
			"environment":        req.Context.Environment,
			"service_names":      req.Context.ServiceNames,
			"resolved_scope":     req.Context.ResolvedScope,
		}
	}

	var resp *ChatResponse
	var err error

	// Local mock mode returns marked simulated provider responses. Production
	// refuses startup without Vertex AI and always runs through the ADK agent.
	if o.mockMode || o.llm == nil {
		resp, err = o.handleMockChat(ctx, sessionID, req)
	} else {
		resp, err = o.handleADKChat(ctx, sessionID, req)
	}

	if err != nil {
		return nil, err
	}

	// Persist the conversation turn before returning it to the caller.
	if saveErr := o.memoryStore.SaveMessage(ctx, sessionID, userMsg); saveErr != nil {
		return nil, fmt.Errorf("failed to persist user message: %w", saveErr)
	}

	assistantMsg := memory.ChatMessage{
		ID:        fmt.Sprintf("mod-%d", time.Now().UnixNano()),
		Role:      "model",
		Content:   resp.Response,
		Timestamp: time.Now().UTC(),
	}
	if saveErr := o.memoryStore.SaveMessage(ctx, sessionID, assistantMsg); saveErr != nil {
		return nil, fmt.Errorf("failed to persist model response: %w", saveErr)
	}

	return resp, nil
}

func (o *Orchestrator) handleADKChat(ctx context.Context, sessionID string, req ChatRequest) (*ChatResponse, error) {
	history, err := o.memoryStore.GetHistory(ctx, sessionID, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to load conversation history: %w", err)
	}
	toolsForAgent, err := o.toolsExecutor.ADKTools()
	if err != nil {
		return nil, fmt.Errorf("create ADK read-only tools: %w", err)
	}
	sysPrompt := BaseSystemPrompt + FormatScreenContext(req.Context)
	root, err := llmagent.New(llmagent.Config{
		Name:                  "ai_ops_copilot",
		Description:           "Read-only SRE assistant for Grafana and five Google Cloud projects.",
		Model:                 o.llm,
		Instruction:           sysPrompt,
		Tools:                 toolsForAgent,
		GenerateContentConfig: &genai.GenerateContentConfig{MaxOutputTokens: 4096},
	})
	if err != nil {
		return nil, fmt.Errorf("create ADK agent: %w", err)
	}
	sessionService := session.InMemoryService()
	userID := req.UserID
	if userID == "" {
		userID = "anonymous"
	}
	created, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName: "ai-ops-copilot", UserID: userID, SessionID: sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("create ADK turn session: %w", err)
	}
	for _, message := range history {
		if strings.TrimSpace(message.Content) == "" || (message.Role != "user" && message.Role != "model") {
			continue
		}
		event := session.NewEvent(ctx, "history")
		if message.Role == "user" {
			event.Author = "user"
			event.LLMResponse.Content = genai.NewContentFromText(message.Content, genai.RoleUser)
		} else {
			event.Author = "ai_ops_copilot"
			event.LLMResponse.Content = genai.NewContentFromText(message.Content, genai.RoleModel)
		}
		if err := sessionService.AppendEvent(ctx, created.Session, event); err != nil {
			return nil, fmt.Errorf("seed ADK conversation history: %w", err)
		}
	}
	runtime, err := runner.New(runner.Config{
		AppName: "ai-ops-copilot", Agent: root, SessionService: sessionService,
		ArtifactService: artifact.InMemoryService(), MemoryService: adkmemory.InMemoryService(),
		AutoCreateSession: false,
	})
	if err != nil {
		return nil, fmt.Errorf("create ADK runner: %w", err)
	}

	var invokedTools []InvokedTool
	var finalText strings.Builder
	message := genai.NewContentFromText(req.Message, genai.RoleUser)
	for event, runErr := range runtime.Run(ctx, userID, sessionID, message, adkagent.RunConfig{
		StreamingMode: adkagent.StreamingModeNone,
	}) {
		if runErr != nil {
			return nil, fmt.Errorf("ADK agent run failed: %w", runErr)
		}
		if event == nil || event.LLMResponse.Content == nil {
			continue
		}
		for _, part := range event.LLMResponse.Content.Parts {
			if part.FunctionCall != nil {
				invokedTools = append(invokedTools, InvokedTool{
					Name: part.FunctionCall.Name, Args: part.FunctionCall.Args,
				})
			}
			if part.FunctionResponse != nil {
				for i := len(invokedTools) - 1; i >= 0; i-- {
					if invokedTools[i].Name == part.FunctionResponse.Name && invokedTools[i].Result == nil {
						invokedTools[i].Result = part.FunctionResponse.Response
						break
					}
				}
			}
			if part.Text != "" && event.IsFinalResponse() {
				finalText.WriteString(part.Text)
			}
		}
	}
	response := strings.TrimSpace(finalText.String())
	if response == "" {
		response = fallbackToolSummary(req.Message, invokedTools)
	}
	return &ChatResponse{Response: response, SessionID: sessionID, ToolCalls: invokedTools}, nil
}

func fallbackToolSummary(message string, calls []InvokedTool) string {
	english := likelyEnglish(message)
	if len(calls) == 0 {
		if english {
			return "I couldn't produce a conclusive answer. No telemetry was retrieved; please retry or narrow the question."
		}
		return "Não consegui produzir uma resposta conclusiva. Nenhuma telemetria foi obtida; tente novamente ou especifique o escopo."
	}
	var b strings.Builder
	if english {
		b.WriteString("I couldn't synthesize a conclusive answer. The following tools returned data:\n")
	} else {
		b.WriteString("Não consegui sintetizar uma resposta conclusiva. Estas ferramentas retornaram dados:\n")
	}
	for _, call := range calls {
		if failure, ok := call.Result["error"].(string); ok {
			fmt.Fprintf(&b, "- %s: %s\n", call.Name, failure)
		} else if english {
			fmt.Fprintf(&b, "- %s: data retrieved; no summary was generated.\n", call.Name)
		} else {
			fmt.Fprintf(&b, "- %s: dados recuperados; o resumo não foi gerado.\n", call.Name)
		}
	}
	return b.String()
}

func likelyEnglish(message string) bool {
	lower := " " + strings.ToLower(message) + " "
	for _, marker := range []string{" the ", " is ", " are ", " what ", " how ", " show ", " cost ", " errors ", " tokens ", " last ", " project ", " please "} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func (o *Orchestrator) handleMockChat(ctx context.Context, sessionID string, req ChatRequest) (*ChatResponse, error) {
	lowerMsg := strings.ToLower(req.Message)
	scope := ResolveQueryScope(req.Message, req.Context)
	var name string
	args := map[string]any{}
	switch {
	case strings.Contains(lowerMsg, "bill") || strings.Contains(lowerMsg, "custo") || strings.Contains(lowerMsg, "fatura") || strings.Contains(lowerMsg, "billing"):
		name, args = tools.ToolQueryBillingCosts, map[string]any{}
		if scope.Mode == "project" {
			args["project_id"] = scope.EffectiveProject
		}
	case req.Context != nil && req.Context.ActivePanel != "":
		name, args = tools.ToolGetDashboardMetrics, map[string]any{"panel_id": req.Context.ActivePanel, "time_range": "selected"}
	case strings.Contains(lowerMsg, "log") || strings.Contains(lowerMsg, "erro") || strings.Contains(lowerMsg, "error"):
		name, args = tools.ToolQueryCloudLogging, map[string]any{"project_id": scope.EffectiveProject, "severity": "ERROR", "limit": 5}
	case strings.Contains(lowerMsg, "token"):
		project := scope.EffectiveProject
		if scope.Mode != "project" {
			project = "enterprise-ai-gateway"
		}
		name, args = tools.ToolQueryCloudMonitoring, map[string]any{
			"project_id": project, "metric_type": "aiplatform.googleapis.com/publisher/online_serving/token_count",
			"time_range": "24h", "aligner": "sum", "reducer": "sum",
		}
	case scope.Mode == "project":
		name, args = tools.ToolQueryCloudMonitoring, map[string]any{
			"project_id": scope.EffectiveProject, "metric_type": "run.googleapis.com/request_count",
			"time_range": "1h", "aligner": "rate", "reducer": "sum",
			"group_by": []any{"resource.label.service_name"},
		}
	default:
		name, args = tools.ToolGetFirestoreDigest, map[string]any{"lookback_hours": 2}
	}

	result, err := o.toolsExecutor.Execute(ctx, name, args)
	if err != nil {
		result = map[string]any{"error": err.Error()}
	}
	invoked := []InvokedTool{{Name: name, Args: args, Result: result}}
	payload, _ := json.MarshalIndent(result, "", "  ")
	var response string
	if likelyEnglish(req.Message) {
		response = "Local development mock mode: these results are simulated and do not describe production.\n\n```json\n" + string(payload) + "\n```"
	} else {
		response = "Modo simulado de desenvolvimento: estes resultados não descrevem a produção.\n\n```json\n" + string(payload) + "\n```"
	}
	return &ChatResponse{Response: response, SessionID: sessionID, ToolCalls: invoked}, nil
}

// GenerateDigest compiles an infrastructure summary across monitored projects and persists it to memory/Firestore.
func (o *Orchestrator) GenerateDigest(ctx context.Context, lookbackHours int) (*tools.DigestSnapshot, error) {
	o.logger.Info("Generating periodic infrastructure digest", "lookback_hours", lookbackHours)

	snapRaw, err := o.toolsExecutor.Execute(ctx, tools.ToolGetFirestoreDigest, map[string]any{
		"lookback_hours": lookbackHours,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate digest snapshot: %w", err)
	}

	summary := ""
	if s, ok := snapRaw["summary"].(string); ok && s != "" && s != "<nil>" {
		summary = s
	} else {
		summary = fmt.Sprintf("No health summary was returned by the snapshot for the last %d hours; status unverified.", lookbackHours)
	}

	snap := &tools.DigestSnapshot{
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		LookbackHours: lookbackHours,
		Summary:       summary,
	}

	// Persist to session store / Firestore
	_ = o.memoryStore.SaveSnapshot(ctx, memory.Snapshot{
		Timestamp:     time.Now().UTC(),
		LookbackHours: lookbackHours,
		Summary:       snap.Summary,
	})

	return snap, nil
}
