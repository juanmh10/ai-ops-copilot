package tools

import (
	"context"
	"testing"

	"google.golang.org/adk/v2/agent"
)

type recordingBillingClient struct{ query BillingQuery }

func (b *recordingBillingClient) QueryCosts(_ context.Context, query BillingQuery) (*BillingResult, error) {
	b.query = query
	return &BillingResult{Lines: []BillingLine{{InvoiceMonth: "202601", ProjectID: "total-monitored", Currency: "USD", GrossCost: 10, CreditsApplied: 2, NetCost: 8}}}, nil
}

type recordingMonitoringClient struct{ query MonitoringQuery }

func (m *recordingMonitoringClient) QueryMetrics(_ context.Context, query MonitoringQuery) (*MonitoringResult, error) {
	m.query = query
	return &MonitoringResult{ProjectID: query.ProjectID, MetricType: query.MetricType, TimeRange: query.TimeRange, Query: query}, nil
}

func TestADKToolsExposeOnlyReadOnlyOperations(t *testing.T) {
	executor := NewExecutor(NewMockGrafanaClient(), NewMockMonitoringClient(), NewMockLoggingClient(), NewMockDigestClient(), nil)
	decls, err := executor.ADKTools()
	if err != nil {
		t.Fatalf("create ADK tools: %v", err)
	}
	if len(decls) != 5 {
		t.Fatalf("expected five read-only ADK tools, got %d", len(decls))
	}

	expectedTools := map[string]bool{
		ToolGetDashboardMetrics:  false,
		ToolQueryCloudMonitoring: false,
		ToolQueryCloudLogging:    false,
		ToolGetFirestoreDigest:   false,
		ToolQueryBillingCosts:    false,
	}

	for _, d := range decls {
		if _, ok := expectedTools[d.Name()]; ok {
			expectedTools[d.Name()] = true
		} else {
			t.Errorf("unexpected ADK tool: %s", d.Name())
		}
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("missing expected tool declaration: %s", name)
		}
	}
}

func TestBillingADKToolAcceptsNullOptionalArguments(t *testing.T) {
	billing := &recordingBillingClient{}
	executor := NewExecutor(NewMockGrafanaClient(), NewMockMonitoringClient(), NewMockLoggingClient(), NewMockDigestClient(), nil, billing)
	decls, err := executor.ADKTools()
	if err != nil {
		t.Fatalf("create ADK tools: %v", err)
	}

	var billingTool interface {
		Run(agent.Context, any) (map[string]any, error)
	}
	for _, decl := range decls {
		if decl.Name() == ToolQueryBillingCosts {
			billingTool, _ = decl.(interface {
				Run(agent.Context, any) (map[string]any, error)
			})
			break
		}
	}
	if billingTool == nil {
		t.Fatal("QueryBillingCosts does not expose an invokable ADK function tool")
	}

	result, err := billingTool.Run(&agent.ContextMock{}, map[string]any{
		"project_id": nil,
		"months":     nil,
	})
	if err != nil {
		t.Fatalf("invoke billing tool with nullable optional arguments: %v", err)
	}
	if billing.query.ProjectID != "" || billing.query.Months != 0 {
		t.Errorf("null optional arguments must select all projects and months, got %+v", billing.query)
	}
	if result["lines"] == nil {
		t.Errorf("expected billing result lines, got %#v", result)
	}

	_, err = billingTool.Run(&agent.ContextMock{}, map[string]any{
		"project_id": []any{"enterprise-ai-analytics"},
		"months":     nil,
	})
	if err == nil {
		t.Fatal("expected invalid project_id type to fail schema validation")
	}
}

func TestExecutor_ReadOnlyTools(t *testing.T) {
	ctx := context.Background()
	executor := NewExecutor(
		NewMockGrafanaClient(),
		NewMockMonitoringClient(),
		NewMockLoggingClient(),
		NewMockDigestClient(),
		nil,
	)

	// Test 1: GetDashboardMetrics
	t.Run("GetDashboardMetrics", func(t *testing.T) {
		panelCtx := WithDashboardQueryContext(ctx, DashboardQueryContext{
			DashboardUID: "aiops-core-prod", TimeFrom: "now-1h", TimeTo: "now",
		})
		res, err := executor.Execute(panelCtx, ToolGetDashboardMetrics, map[string]any{
			"panel_id":   "12",
			"time_range": "selected",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res["panel_id"] != "12" {
			t.Errorf("expected panel_id '12', got %v", res["panel_id"])
		}
		if res["status"] != "mock" {
			t.Errorf("expected explicit mock status, got %v", res["status"])
		}

		// Missing required param
		_, err = executor.Execute(ctx, ToolGetDashboardMetrics, map[string]any{})
		if err == nil {
			t.Error("expected error for missing panel_id, got nil")
		}
	})

	// Test 6: QueryBillingCosts
	t.Run("QueryBillingCosts", func(t *testing.T) {
		billing := &recordingBillingClient{}
		billingExecutor := NewExecutor(NewMockGrafanaClient(), NewMockMonitoringClient(), NewMockLoggingClient(), NewMockDigestClient(), nil, billing)
		res, err := billingExecutor.Execute(ctx, ToolQueryBillingCosts, map[string]any{"months": 0})
		if err != nil {
			t.Fatalf("query billing: %v", err)
		}
		if billing.query.Months != 0 || res["lines"] == nil {
			t.Fatalf("expected all available invoice months, query=%+v result=%+v", billing.query, res)
		}
	})

	// Test 2: QueryCloudMonitoring
	t.Run("QueryCloudMonitoring", func(t *testing.T) {
		res, err := executor.Execute(ctx, ToolQueryCloudMonitoring, map[string]any{
			"project_id":        "enterprise-core-prod",
			"metric_type":       "run.googleapis.com/request_count",
			"time_range":        "1h",
			"aligner":           "rate",
			"reducer":           "sum",
			"group_by":          []any{"resource.label.service_name"},
			"series_limit":      25,
			"alignment_seconds": 60,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res["project_id"] != "enterprise-core-prod" {
			t.Errorf("expected project_id 'enterprise-core-prod', got %v", res["project_id"])
		}
		query, ok := res["query"].(map[string]any)
		if !ok || query["aligner"] != "rate" || query["reducer"] != "sum" {
			t.Errorf("expected granular query metadata, got %#v", res["query"])
		}

		// Missing required params
		_, err = executor.Execute(ctx, ToolQueryCloudMonitoring, map[string]any{
			"project_id": "enterprise-core-prod",
		})
		if err == nil {
			t.Error("expected error for missing metric_type, got nil")
		}

		_, err = executor.Execute(ctx, ToolQueryCloudMonitoring, map[string]any{
			"project_id":  "outside-project",
			"metric_type": "run.googleapis.com/request_count",
		})
		if err == nil {
			t.Error("expected monitored-project guardrail error, got nil")
		}
	})

	t.Run("NormalizeGrafanaMetricAliases", func(t *testing.T) {
		monitoring := &recordingMonitoringClient{}
		monitoringExecutor := NewExecutor(NewMockGrafanaClient(), monitoring, NewMockLoggingClient(), NewMockDigestClient(), nil)
		_, err := monitoringExecutor.Execute(ctx, ToolQueryCloudMonitoring, map[string]any{
			"project_id":  "enterprise-core-prod",
			"metric_type": "http/server/request_count",
			"time_range":  "1h",
			"filter":      `response_code=~"5.."`,
		})
		if err != nil {
			t.Fatalf("normalize Grafana metric alias: %v", err)
		}
		if monitoring.query.MetricType != "run.googleapis.com/request_count" {
			t.Errorf("metric_type was not normalized: %q", monitoring.query.MetricType)
		}
		if monitoring.query.Filter != `metric.label.response_code_class = "5xx"` {
			t.Errorf("5xx filter was not normalized: %q", monitoring.query.Filter)
		}
	})

	// Test 3: QueryCloudLogging
	t.Run("QueryCloudLogging", func(t *testing.T) {
		res, err := executor.Execute(ctx, ToolQueryCloudLogging, map[string]any{
			"project_id": "enterprise-telemetry-prod",
			"severity":   "ERROR",
			"limit":      5,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res["project_id"] != "enterprise-telemetry-prod" {
			t.Errorf("expected project_id 'enterprise-telemetry-prod', got %v", res["project_id"])
		}
		logs, ok := res["logs"].([]any)
		if !ok || len(logs) == 0 {
			t.Error("expected non-empty logs array")
		}

		// Missing project_id
		_, err = executor.Execute(ctx, ToolQueryCloudLogging, map[string]any{})
		if err == nil {
			t.Error("expected error for missing project_id, got nil")
		}
	})

	// Test 4: GetFirestoreDigest
	t.Run("GetFirestoreDigest", func(t *testing.T) {
		res, err := executor.Execute(ctx, ToolGetFirestoreDigest, map[string]any{
			"lookback_hours": 2,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res["summary"] == nil {
			t.Error("expected summary in digest result")
		}
	})

	// Test 5: Guardrail rejection for forbidden mutations (RULE 1)
	t.Run("Guardrail_ForbiddenActions", func(t *testing.T) {
		forbidden := []string{
			"ApplyScale",
			"RestartService",
			"DeleteDatabase",
			"UpdateCloudRun",
			"DeployNewRevision",
			"UnknownTool",
		}

		for _, tool := range forbidden {
			_, err := executor.Execute(ctx, tool, map[string]any{"service": "portal-api"})
			if err == nil {
				t.Errorf("expected guardrail to block forbidden action %s, but execution succeeded", tool)
			}
		}
	})
}

func TestParseTimeRange(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"now-1h", "1h0m0s"},
		{"now-3h", "3h0m0s"},
		{"now-6h", "6h0m0s"},
		{"now-30m", "30m0s"},
		{"24h", "24h0m0s"},
		{"1d", "24h0m0s"},
		{"3d", "72h0m0s"},
		{"7d", "168h0m0s"},
		{"", "1h0m0s"},
		{"unknown", "1h0m0s"},
	}

	for _, c := range cases {
		d := parseTimeRange(c.input)
		if d.String() != c.expected {
			t.Errorf("for input %q: expected %s, got %s", c.input, c.expected, d.String())
		}
	}
}
