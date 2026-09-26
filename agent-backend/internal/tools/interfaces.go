package tools

import (
	"context"

	"google.golang.org/adk/v2/tool"
)

// ToolExecutor is the interface for executing read-only tools invoked by the Gemini LLM.
type ToolExecutor interface {
	Execute(ctx context.Context, name string, args map[string]any) (map[string]any, error)
	ADKTools() ([]tool.Tool, error)
}

// GrafanaClient is an interface for querying the Grafana local API.
type GrafanaClient interface {
	GetPanelMetrics(ctx context.Context, scope DashboardQueryContext, panelID, timeRange string) (*MetricsResult, error)
}

// BillingClient runs bounded read-only queries against the configured export.
type BillingClient interface {
	QueryCosts(ctx context.Context, query BillingQuery) (*BillingResult, error)
}

// MonitoringClient is an interface for querying Google Cloud Monitoring.
type MonitoringClient interface {
	QueryMetrics(ctx context.Context, query MonitoringQuery) (*MonitoringResult, error)
}

// LoggingClient is an interface for querying Google Cloud Logging.
type LoggingClient interface {
	QueryLogs(ctx context.Context, projectID, severity, filter string, limit int) ([]LogEntry, error)
}

// DigestClient is an interface for retrieving pre-computed telemetry snapshots.
type DigestClient interface {
	GetDigest(ctx context.Context, lookbackHours int) (*DigestSnapshot, error)
}
