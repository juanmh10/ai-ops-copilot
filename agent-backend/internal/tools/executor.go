package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// Executor coordinates and safely executes read-only tools.
type Executor struct {
	grafana    GrafanaClient
	monitoring MonitoringClient
	logging    LoggingClient
	digest     DigestClient
	billing    BillingClient
	logger     *slog.Logger
}

// NewExecutor creates a new Executor with the given providers.
func NewExecutor(
	grafana GrafanaClient,
	monitoring MonitoringClient,
	logging LoggingClient,
	digest DigestClient,
	logger *slog.Logger,
	billingClients ...BillingClient,
) *Executor {
	if logger == nil {
		logger = slog.Default()
	}
	var billing BillingClient
	if len(billingClients) > 0 {
		billing = billingClients[0]
	}
	return &Executor{
		grafana:    grafana,
		monitoring: monitoring,
		logging:    logging,
		digest:     digest,
		billing:    billing,
		logger:     logger,
	}
}

type panelToolArgs struct {
	PanelID   string `json:"panel_id"`
	TimeRange string `json:"time_range,omitempty"`
}

type monitoringToolArgs struct {
	ProjectID        string   `json:"project_id"`
	MetricType       string   `json:"metric_type"`
	TimeRange        string   `json:"time_range,omitempty"`
	Filter           string   `json:"filter,omitempty"`
	Aligner          string   `json:"aligner,omitempty"`
	Reducer          string   `json:"reducer,omitempty"`
	AlignmentSeconds int      `json:"alignment_seconds,omitempty"`
	GroupBy          []string `json:"group_by,omitempty"`
	SeriesLimit      int      `json:"series_limit,omitempty"`
}

type loggingToolArgs struct {
	ProjectID string `json:"project_id"`
	Severity  string `json:"severity,omitempty"`
	Filter    string `json:"filter,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type digestToolArgs struct {
	LookbackHours int `json:"lookback_hours,omitempty"`
}

type billingToolArgs struct {
	ProjectID string `json:"project_id,omitempty"`
	Months    int    `json:"months,omitempty"`
}

// ADKTools returns only allowlisted read-only operations with typed input schemas.
func (e *Executor) ADKTools() ([]tool.Tool, error) {
	items := make([]tool.Tool, 0, 5)
	add := func(item tool.Tool, err error) error {
		if err != nil {
			return err
		}
		items = append(items, item)
		return nil
	}
	item, err := functiontool.New(functiontool.Config{
		Name:        ToolGetDashboardMetrics,
		Description: "Read metric series data from the active Grafana panel, adhering to dashboard context, active filters, and selected time range. If no panel is active, query Cloud Monitoring.",
	}, func(ctx agent.Context, args panelToolArgs) (map[string]any, error) {
		return e.executeForADK(ctx, ToolGetDashboardMetrics, args)
	})
	if err := add(item, err); err != nil {
		return nil, err
	}

	item, err = functiontool.New(functiontool.Config{
		Name:        ToolQueryCloudMonitoring,
		Description: "Query read-only Cloud Monitoring metrics across supervised GCP projects. Use exact metric types matching Grafana panels: run.googleapis.com/request_count, run.googleapis.com/request_latencies, run.googleapis.com/container/instance_count, aiplatform.googleapis.com/publisher/online_serving/token_count, and aiplatform.googleapis.com/publisher/online_serving/model_invocation_count. For Cloud Run errors, use metric.label.response_code_class = \"5xx\"; filters adhere to Cloud Monitoring syntax, not PromQL. Adhere to dashboard resolved_scope and apply appropriate alignment and aggregations.",
	}, func(ctx agent.Context, args monitoringToolArgs) (map[string]any, error) {
		return e.executeForADK(ctx, ToolQueryCloudMonitoring, args)
	})
	if err := add(item, err); err != nil {
		return nil, err
	}

	item, err = functiontool.New(functiontool.Config{
		Name:        ToolQueryCloudLogging,
		Description: "Query read-only Cloud Logging entries to diagnose errors and infrastructure events across supervised projects.",
	}, func(ctx agent.Context, args loggingToolArgs) (map[string]any, error) {
		return e.executeForADK(ctx, ToolQueryCloudLogging, args)
	})
	if err := add(item, err); err != nil {
		return nil, err
	}

	item, err = functiontool.New(functiontool.Config{
		Name:        ToolGetFirestoreDigest,
		Description: "Read the most recent infrastructure health and telemetry snapshot stored in Cloud Firestore.",
	}, func(ctx agent.Context, args digestToolArgs) (map[string]any, error) {
		return e.executeForADK(ctx, ToolGetFirestoreDigest, args)
	})
	if err := add(item, err); err != nil {
		return nil, err
	}

	item, err = functiontool.New(functiontool.Config{
		Name:        ToolQueryBillingCosts,
		Description: "Query actual cloud costs across all available invoice months from BigQuery Cloud Billing standard export. Without project_id returns cost breakdowns per project, total across supervised projects, account-wide total, and projects with zero billing rows; with project_id filters to the specific project. Returns invoice month, currency, gross cost, credits applied, net exported cost, and last export timestamp.",
		InputSchema: billingToolInputSchema(),
	}, func(ctx agent.Context, args billingToolArgs) (map[string]any, error) {
		return e.executeForADK(ctx, ToolQueryBillingCosts, args)
	})
	if err := add(item, err); err != nil {
		return nil, err
	}
	return items, nil
}

func billingToolInputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"project_id": {
				Types:       []string{"string", "null"},
				Description: "Optional monitored project id. Use null or omit it to query all monitored projects and the whole billing account.",
			},
			"months": {
				Types:       []string{"integer", "null"},
				Description: "Optional number of latest invoice months. Use null or 0 for all available months.",
			},
		},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

func (e *Executor) executeForADK(ctx context.Context, name string, args any) (map[string]any, error) {
	encoded, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	var values map[string]any
	if err := json.Unmarshal(encoded, &values); err != nil {
		return nil, err
	}
	result, err := e.Execute(ctx, name, values)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}
	return result, nil
}

var grafanaDashboardUIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
var grafanaPanelIDPattern = regexp.MustCompile(`^[0-9]{1,12}$`)
var grafanaRangePattern = regexp.MustCompile(`^now-[1-9][0-9]{0,3}[smhdw]$`)
var responseCodeRegexFilter = regexp.MustCompile(`(?i)(?:metric\.label\.)?response_code(?:_class)?\s*=~\s*["']?([45](?:\.\.|xx))["']?`)

// Execute validates and routes the tool execution strictly to read-only providers.
func (e *Executor) Execute(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	e.logger.Info("Executing read-only tool", "tool", name, "args", args)

	switch name {
	case ToolGetDashboardMetrics:
		panelID := getString(args, "panel_id")
		if !grafanaPanelIDPattern.MatchString(panelID) {
			return nil, errors.New("missing or invalid active panel_id")
		}
		scope := dashboardQueryContextFrom(ctx)
		if !grafanaDashboardUIDPattern.MatchString(scope.DashboardUID) {
			return nil, errors.New("missing or invalid active dashboard uid")
		}
		timeRange := getString(args, "time_range")
		if timeRange != "" && timeRange != "selected" && !grafanaRangePattern.MatchString(timeRange) {
			return nil, errors.New("invalid Grafana time range")
		}

		res, err := e.grafana.GetPanelMetrics(ctx, scope, panelID, timeRange)
		if err != nil {
			return nil, fmt.Errorf("error executing GetDashboardMetrics: %w", err)
		}
		return toMap(res)

	case ToolQueryCloudMonitoring:
		projectID := getString(args, "project_id")
		if projectID == "" {
			return nil, errors.New("missing required argument 'project_id'")
		}
		if !isAllowedProject(projectID) {
			return nil, fmt.Errorf("project_id %q is outside the monitored scope", projectID)
		}
		metricType := normalizeMonitoringMetricType(getString(args, "metric_type"))
		if metricType == "" {
			return nil, errors.New("missing required argument 'metric_type'")
		}
		if strings.ContainsAny(metricType, "\"'\n\r") || !strings.Contains(metricType, ".googleapis.com/") {
			return nil, errors.New("invalid metric_type")
		}
		timeRange := getString(args, "time_range")
		if timeRange == "" {
			timeRange = "1h"
		}
		filter := normalizeMonitoringFilter(metricType, strings.TrimSpace(getString(args, "filter")))
		if len(filter) > 1000 {
			return nil, errors.New("filter exceeds 1000 characters")
		}
		groupBy, err := getStringSlice(args, "group_by")
		if err != nil {
			return nil, err
		}
		for _, field := range groupBy {
			if !validGroupBy(field) {
				return nil, fmt.Errorf("invalid group_by field %q", field)
			}
		}
		alignmentSeconds := getInt(args, "alignment_seconds", 300)
		if alignmentSeconds < 60 || alignmentSeconds > 86400 {
			alignmentSeconds = 300
		}
		seriesLimit := getInt(args, "series_limit", 50)
		if seriesLimit < 1 || seriesLimit > 100 {
			seriesLimit = 50
		}
		query := MonitoringQuery{
			ProjectID: projectID, MetricType: metricType, TimeRange: timeRange,
			Filter: filter, Aligner: getString(args, "aligner"), Reducer: getString(args, "reducer"),
			AlignmentSeconds: alignmentSeconds, GroupBy: groupBy, SeriesLimit: seriesLimit,
		}

		res, err := e.monitoring.QueryMetrics(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("error executing QueryCloudMonitoring: %w", err)
		}
		return toMap(res)

	case ToolQueryCloudLogging:
		projectID := getString(args, "project_id")
		if projectID == "" {
			return nil, errors.New("missing required argument 'project_id'")
		}
		if !isAllowedProject(projectID) {
			return nil, fmt.Errorf("project_id %q is outside the monitored scope", projectID)
		}
		severity := getString(args, "severity")
		if severity == "" {
			severity = "ERROR"
		}
		filter := getString(args, "filter")
		limit := getInt(args, "limit", 10)
		if limit <= 0 || limit > 50 {
			limit = 10
		}

		logs, err := e.logging.QueryLogs(ctx, projectID, severity, filter, limit)
		if err != nil {
			return nil, fmt.Errorf("error executing QueryCloudLogging: %w", err)
		}
		return toMap(map[string]any{
			"project_id": projectID,
			"severity":   severity,
			"count":      len(logs),
			"logs":       logs,
		})

	case ToolGetFirestoreDigest:
		lookback := getInt(args, "lookback_hours", 2)
		if lookback <= 0 {
			lookback = 2
		}

		snap, err := e.digest.GetDigest(ctx, lookback)
		if err != nil {
			return nil, fmt.Errorf("error executing GetFirestoreDigest: %w", err)
		}
		return toMap(snap)

	case ToolQueryBillingCosts:
		if e.billing == nil {
			return nil, errors.New("Cloud Billing export is unavailable")
		}
		projectID := strings.TrimSpace(getString(args, "project_id"))
		if projectID != "" && !isAllowedProject(projectID) {
			return nil, fmt.Errorf("project_id %q is outside the monitored scope", projectID)
		}
		months := getInt(args, "months", 0)
		if months < 0 {
			months = 0
		}
		if months > 120 {
			months = 120
		}
		result, err := e.billing.QueryCosts(ctx, BillingQuery{ProjectID: projectID, Months: months})
		if err != nil {
			return nil, fmt.Errorf("error executing QueryBillingCosts: %w", err)
		}
		return toMap(result)

	default:
		// Guardrail: Explicitly reject any unapproved action or write attempt
		e.logger.Warn("Blocked attempt to execute unauthorized action", "tool", name)
		return nil, fmt.Errorf("action '%s' is not permitted. Only approved read-only tools are allowed in MVP (RULE 1)", name)
	}
}

func normalizeMonitoringMetricType(metricType string) string {
	trimmed := strings.TrimSpace(metricType)
	switch strings.ToLower(trimmed) {
	case "http/server/request_count", "run/request_count", "cloudrun/request_count", "request_count":
		return "run.googleapis.com/request_count"
	case "http/server/request_latencies", "run/request_latencies", "request_latencies":
		return "run.googleapis.com/request_latencies"
	case "run/container/instance_count", "container/instance_count", "instance_count":
		return "run.googleapis.com/container/instance_count"
	default:
		return trimmed
	}
}

func normalizeMonitoringFilter(metricType, filter string) string {
	if metricType != "run.googleapis.com/request_count" || filter == "" {
		return filter
	}
	match := responseCodeRegexFilter.FindStringSubmatch(filter)
	if len(match) != 2 {
		return filter
	}
	class := strings.TrimRight(match[1], ".")
	if len(class) == 1 {
		class += "xx"
	}
	replacement := `metric.label.response_code_class = "` + class + `"`
	return strings.Replace(filter, match[0], replacement, 1)
}

func getString(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func getInt(args map[string]any, key string, def int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		case json.Number:
			if i, err := n.Int64(); err == nil {
				return int(i)
			}
		}
	}
	return def
}

func getStringSlice(args map[string]any, key string) ([]string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		if strings, ok := value.([]string); ok {
			return strings, nil
		}
		return nil, fmt.Errorf("argument %q must be an array of strings", key)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("argument %q must contain only strings", key)
		}
		result = append(result, text)
	}
	return result, nil
}

func validGroupBy(field string) bool {
	if !(strings.HasPrefix(field, "resource.label.") || strings.HasPrefix(field, "metric.label.")) {
		return false
	}
	for _, char := range field {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func isAllowedProject(projectID string) bool {
	switch projectID {
	case "enterprise-core-prod", "enterprise-saas-staging", "enterprise-ai-analytics", "enterprise-telemetry-prod", "enterprise-ai-gateway":
		return true
	default:
		return false
	}
}

func toMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var res map[string]any
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return res, nil
}
