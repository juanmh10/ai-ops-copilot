package tools

import (
	"context"
	"fmt"
	"time"
)

// MockGrafanaClient provides simulated metrics for Grafana dashboard panels.
type MockGrafanaClient struct{}

func NewMockGrafanaClient() *MockGrafanaClient {
	return &MockGrafanaClient{}
}

func (m *MockGrafanaClient) GetPanelMetrics(ctx context.Context, scope DashboardQueryContext, panelID, timeRange string) (*MetricsResult, error) {
	return &MetricsResult{
		PanelID: panelID, Dashboard: scope.DashboardUID,
		TimeRange: timeRange, Data: map[string]any{"notice": "simulated local result", "data_points": []any{}},
		Status: "mock",
	}, nil
}

// MockMonitoringClient provides simulated Cloud Monitoring responses.
type MockMonitoringClient struct{}

func NewMockMonitoringClient() *MockMonitoringClient {
	return &MockMonitoringClient{}
}

func (m *MockMonitoringClient) QueryMetrics(ctx context.Context, query MonitoringQuery) (*MonitoringResult, error) {
	now := time.Now().UTC()
	var series []MetricSeries
	if query.MetricType == "aiplatform.googleapis.com/publisher/online_serving/token_count" {
		return &MonitoringResult{
			ProjectID: query.ProjectID, MetricType: query.MetricType, TimeRange: query.TimeRange, Query: query,
			Series: []MetricSeries{
				{ResourceLabels: map[string]string{"model_user_id": "gemini-3.8-flash"}, Points: []DataPoint{{Timestamp: now.Format(time.RFC3339), Value: 2200}}},
				{ResourceLabels: map[string]string{"model_user_id": "gemini-3.5-flash-lite"}, Points: []DataPoint{{Timestamp: now.Format(time.RFC3339), Value: 800}}},
			},
			Summary: "Simulated Vertex AI token metrics for local development.",
		}, nil
	}

	switch query.ProjectID {
	case "enterprise-core-prod":
		series = []MetricSeries{
			{
				ResourceName: "portal-api",
				Points: []DataPoint{
					{Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339), Value: 145},
					{Timestamp: now.Format(time.RFC3339), Value: 160},
				},
			},
			{
				ResourceName: "auth-gateway-api",
				Points: []DataPoint{
					{Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339), Value: 85},
					{Timestamp: now.Format(time.RFC3339), Value: 92},
				},
			},
		}
	case "enterprise-telemetry-prod":
		series = []MetricSeries{
			{
				ResourceName: "telemetry-ingest-prod",
				Points: []DataPoint{
					{Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339), Value: 420},
					{Timestamp: now.Format(time.RFC3339), Value: 465},
				},
			},
		}
	case "enterprise-saas-staging":
		series = []MetricSeries{
			{
				ResourceName: "stg-saas-api",
				Points: []DataPoint{
					{Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339), Value: 30},
					{Timestamp: now.Format(time.RFC3339), Value: 35},
				},
			},
		}
	case "enterprise-ai-analytics":
		series = []MetricSeries{
			{
				ResourceName: "ai-service-api",
				Points: []DataPoint{
					{Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339), Value: 512},
					{Timestamp: now.Format(time.RFC3339), Value: 530},
				},
			},
		}
	default:
		series = []MetricSeries{
			{
				ResourceName: "generic-service",
				Points: []DataPoint{
					{Timestamp: now.Format(time.RFC3339), Value: 10},
				},
			},
		}
	}

	return &MonitoringResult{
		ProjectID:  query.ProjectID,
		MetricType: query.MetricType,
		TimeRange:  query.TimeRange,
		Query:      query,
		Series:     series,
		Summary:    fmt.Sprintf("Simulated metrics of %s for %s over window %s", query.MetricType, query.ProjectID, query.TimeRange),
	}, nil
}

// MockLoggingClient provides simulated Cloud Logging records.
type MockLoggingClient struct{}

func NewMockLoggingClient() *MockLoggingClient {
	return &MockLoggingClient{}
}

func (m *MockLoggingClient) QueryLogs(ctx context.Context, projectID, severity, filter string, limit int) ([]LogEntry, error) {
	now := time.Now().UTC()
	entries := []LogEntry{
		{
			Timestamp: now.Add(-10 * time.Minute).Format(time.RFC3339),
			Severity:  severity,
			Resource:  fmt.Sprintf("projects/%s/services/api-core", projectID),
			Payload:   fmt.Sprintf("[HEALTHCHECK] 200 OK - All subsystems operational in %s", projectID),
		},
	}
	if severity == "ERROR" || severity == "CRITICAL" {
		entries = append(entries, LogEntry{
			Timestamp: now.Add(-25 * time.Minute).Format(time.RFC3339),
			Severity:  "ERROR",
			Resource:  fmt.Sprintf("projects/%s/services/worker", projectID),
			Payload:   "Transient upstream timeout (504 Gateway Timeout) handled cleanly by retry policy",
		})
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

// MockDigestClient provides simulated pre-computed snapshots.
type MockDigestClient struct{}

func NewMockDigestClient() *MockDigestClient {
	return &MockDigestClient{}
}

func (m *MockDigestClient) GetDigest(ctx context.Context, lookbackHours int) (*DigestSnapshot, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	return &DigestSnapshot{
		Timestamp:     now,
		LookbackHours: lookbackHours,
		Summary:       "All 5 supervised projects operational with zero critical failures. Global error rate < 0.05%.",
		Projects: []ProjectHealth{
			{
				ProjectID:    "enterprise-core-prod",
				Status:       "HEALTHY",
				ErrorRate:    0.01,
				ActiveAlerts: 0,
				ServiceCount: 2,
			},
			{
				ProjectID:    "enterprise-saas-staging",
				Status:       "HEALTHY",
				ErrorRate:    0.00,
				ActiveAlerts: 0,
				ServiceCount: 2,
			},
			{
				ProjectID:    "enterprise-ai-analytics",
				Status:       "HEALTHY",
				ErrorRate:    0.02,
				ActiveAlerts: 0,
				ServiceCount: 1,
			},
			{
				ProjectID:    "enterprise-telemetry-prod",
				Status:       "HEALTHY",
				ErrorRate:    0.03,
				ActiveAlerts: 0,
				ServiceCount: 2,
			},
			{
				ProjectID:    "enterprise-ai-gateway",
				Status:       "HEALTHY",
				ErrorRate:    0.00,
				ActiveAlerts: 0,
				ServiceCount: 0,
			},
		},
	}, nil
}
