package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	logging "cloud.google.com/go/logging/logadmin"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	monitoringpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RealGrafanaClient queries the Grafana HTTP API.
type RealGrafanaClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRealGrafanaClient creates a new Grafana client.
func NewRealGrafanaClient(baseURL string) *RealGrafanaClient {
	return &RealGrafanaClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (g *RealGrafanaClient) GetPanelMetrics(ctx context.Context, scope DashboardQueryContext, panelID, timeRange string) (*MetricsResult, error) {
	if scope.DashboardUID == "" || panelID == "" {
		return nil, errors.New("dashboard uid and active panel id are required")
	}
	if scope.Cookie == "" && scope.Authorization == "" {
		return nil, errors.New("authenticated Grafana session is required")
	}
	var dashboardResponse struct {
		Dashboard map[string]any `json:"dashboard"`
	}
	if err := g.request(ctx, http.MethodGet, "/api/dashboards/uid/"+scope.DashboardUID, scope, nil, &dashboardResponse); err != nil {
		return nil, fmt.Errorf("read Grafana dashboard: %w", err)
	}
	panel, err := findDashboardPanel(dashboardResponse.Dashboard["panels"], panelID)
	if err != nil {
		return nil, err
	}
	targets, ok := panel["targets"].([]any)
	if !ok || len(targets) == 0 {
		return nil, errors.New("active Grafana panel has no query targets")
	}
	if timeRange != "" && timeRange != "selected" {
		scope.TimeFrom, scope.TimeTo = timeRange, "now"
	}
	if scope.TimeFrom == "" {
		scope.TimeFrom = "now-1h"
	}
	if scope.TimeTo == "" {
		scope.TimeTo = "now"
	}
	queryTargets := make([]map[string]any, 0, len(targets))
	for _, raw := range targets {
		target, ok := raw.(map[string]any)
		if !ok || target["hide"] == true {
			continue
		}
		copyTarget := make(map[string]any, len(target)+3)
		for key, value := range target {
			copyTarget[key] = value
		}
		if datasource, exists := panel["datasource"]; exists && copyTarget["datasource"] == nil {
			copyTarget["datasource"] = datasource
		}
		copyTarget["panelId"] = panel["id"]
		copyTarget["maxDataPoints"] = 1500
		queryTargets = append(queryTargets, copyTarget)
	}
	if len(queryTargets) == 0 {
		return nil, errors.New("active Grafana panel has no visible query targets")
	}
	scopedVars := make(map[string]any, len(scope.ActiveFilters))
	for key, value := range scope.ActiveFilters {
		scopedVars[key] = map[string]any{"text": value, "value": value}
	}
	from, to := scope.TimeFrom, scope.TimeTo
	queryBody := map[string]any{
		"from": from, "to": to, "timezone": "browser", "intervalMs": 30000,
		"maxDataPoints": 1500, "scopedVars": scopedVars, "queries": queryTargets,
		"range":        map[string]any{"from": from, "to": to, "raw": map[string]any{"from": from, "to": to}},
		"dashboardUID": scope.DashboardUID, "panelId": panel["id"],
	}
	var queryResponse map[string]any
	if err := g.request(ctx, http.MethodPost, "/api/ds/query", scope, queryBody, &queryResponse); err != nil {
		return nil, fmt.Errorf("query active Grafana panel: %w", err)
	}
	data, ok := queryResponse["results"].(map[string]any)
	if !ok {
		return nil, errors.New("Grafana query returned no results object")
	}
	return &MetricsResult{PanelID: panelID, Dashboard: scope.DashboardUID,
		TimeRange: from + " to " + to, Data: data, Status: "ok"}, nil
}

func (g *RealGrafanaClient) request(ctx context.Context, method, path string, scope DashboardQueryContext, payload any, output any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode Grafana request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(g.baseURL, "/")+path, body)
	if err != nil {
		return err
	}
	if scope.Cookie != "" {
		req.Header.Set("Cookie", scope.Cookie)
	}
	if scope.Authorization != "" {
		req.Header.Set("Authorization", scope.Authorization)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Grafana request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Grafana API returned HTTP %d", resp.StatusCode)
	}
	const maxGrafanaResponseBytes = 2 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGrafanaResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read Grafana response: %w", err)
	}
	if len(data) > maxGrafanaResponseBytes {
		return fmt.Errorf("Grafana response exceeded the %d-byte result limit", maxGrafanaResponseBytes)
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode Grafana response: %w", err)
	}
	return nil
}

func findDashboardPanel(raw any, panelID string) (map[string]any, error) {
	panels, ok := raw.([]any)
	if !ok {
		return nil, errors.New("Grafana dashboard has no panels")
	}
	for _, rawPanel := range panels {
		panel, ok := rawPanel.(map[string]any)
		if !ok {
			continue
		}
		if fmt.Sprint(panel["id"]) == panelID {
			return panel, nil
		}
		if nested, err := findDashboardPanel(panel["panels"], panelID); err == nil {
			return nested, nil
		}
	}
	return nil, fmt.Errorf("panel %q is not part of the active dashboard", panelID)
}

// RealMonitoringClient queries GCP Cloud Monitoring TimeSeries API.
type RealMonitoringClient struct {
	client *monitoring.MetricClient
}

// NewRealMonitoringClient creates a new Cloud Monitoring client.
func NewRealMonitoringClient(ctx context.Context) (*RealMonitoringClient, error) {
	client, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create monitoring metric client: %w", err)
	}
	return &RealMonitoringClient{client: client}, nil
}

func (m *RealMonitoringClient) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

func (m *RealMonitoringClient) QueryMetrics(ctx context.Context, query MonitoringQuery) (*MonitoringResult, error) {
	now := time.Now().UTC()
	duration := parseTimeRange(query.TimeRange)
	startTime := now.Add(-duration)
	alignmentSeconds := query.AlignmentSeconds
	if alignmentSeconds < 60 || alignmentSeconds > 86400 {
		alignmentSeconds = 300
	}
	seriesLimit := query.SeriesLimit
	if seriesLimit < 1 || seriesLimit > 100 {
		seriesLimit = 50
	}
	filter := fmt.Sprintf(`metric.type = "%s"`, query.MetricType)
	if strings.TrimSpace(query.Filter) != "" {
		filter = fmt.Sprintf(`%s AND (%s)`, filter, query.Filter)
	}

	aligner := parseAligner(query.Aligner)
	if strings.Contains(query.MetricType, "latenc") {
		if aligner == monitoringpb.Aggregation_ALIGN_RATE || aligner == monitoringpb.Aggregation_ALIGN_SUM {
			aligner = monitoringpb.Aggregation_ALIGN_PERCENTILE_95
		}
	}
	aggregation := &monitoringpb.Aggregation{
		AlignmentPeriod:  durationpb.New(time.Duration(alignmentSeconds) * time.Second),
		PerSeriesAligner: aligner,
	}
	if reducer := parseReducer(query.Reducer); reducer != monitoringpb.Aggregation_REDUCE_NONE {
		if strings.Contains(query.MetricType, "latenc") && reducer == monitoringpb.Aggregation_REDUCE_SUM {
			reducer = monitoringpb.Aggregation_REDUCE_PERCENTILE_95
		}
		aggregation.CrossSeriesReducer = reducer
		aggregation.GroupByFields = append([]string(nil), query.GroupBy...)
	}

	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", query.ProjectID),
		Filter: filter,
		Interval: &monitoringpb.TimeInterval{
			StartTime: timestamppb.New(startTime),
			EndTime:   timestamppb.New(now),
		},
		View:        monitoringpb.ListTimeSeriesRequest_FULL,
		Aggregation: aggregation,
	}

	it := m.client.ListTimeSeries(ctx, req)
	var series []MetricSeries

	for {
		ts, err := it.Next()
		if errorsIsIteratorDone(err) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading time series from cloud monitoring: %w", err)
		}

		var points []DataPoint
		for _, pt := range ts.GetPoints() {
			val := numericTypedValue(pt.GetValue())
			points = append(points, DataPoint{
				Timestamp: pt.GetInterval().GetEndTime().AsTime().Format(time.RFC3339),
				Value:     val,
			})
		}

		resourceLabels := cloneLabels(ts.GetResource().GetLabels())
		metricLabels := cloneLabels(ts.GetMetric().GetLabels())
		resName := resourceDisplayName(resourceLabels)

		series = append(series, MetricSeries{
			ResourceName:   resName,
			ResourceLabels: resourceLabels,
			MetricLabels:   metricLabels,
			Points:         points,
		})
		if len(series) >= seriesLimit {
			break
		}
	}

	return &MonitoringResult{
		ProjectID:  query.ProjectID,
		MetricType: query.MetricType,
		TimeRange:  query.TimeRange,
		Query:      query,
		Series:     series,
		Summary:    fmt.Sprintf("Found %d series for metric %s", len(series), query.MetricType),
	}, nil
}

func parseAligner(value string) monitoringpb.Aggregation_Aligner {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sum":
		return monitoringpb.Aggregation_ALIGN_SUM
	case "rate":
		return monitoringpb.Aggregation_ALIGN_RATE
	case "max":
		return monitoringpb.Aggregation_ALIGN_MAX
	case "min":
		return monitoringpb.Aggregation_ALIGN_MIN
	case "p50", "percentile_50":
		return monitoringpb.Aggregation_ALIGN_PERCENTILE_50
	case "p95", "percentile_95":
		return monitoringpb.Aggregation_ALIGN_PERCENTILE_95
	case "p99", "percentile_99":
		return monitoringpb.Aggregation_ALIGN_PERCENTILE_99
	case "fraction_true":
		return monitoringpb.Aggregation_ALIGN_FRACTION_TRUE
	default:
		return monitoringpb.Aggregation_ALIGN_MEAN
	}
}

func parseReducer(value string) monitoringpb.Aggregation_Reducer {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sum":
		return monitoringpb.Aggregation_REDUCE_SUM
	case "mean":
		return monitoringpb.Aggregation_REDUCE_MEAN
	case "max":
		return monitoringpb.Aggregation_REDUCE_MAX
	case "min":
		return monitoringpb.Aggregation_REDUCE_MIN
	case "p50", "percentile_50":
		return monitoringpb.Aggregation_REDUCE_PERCENTILE_50
	case "p95", "percentile_95":
		return monitoringpb.Aggregation_REDUCE_PERCENTILE_95
	case "p99", "percentile_99":
		return monitoringpb.Aggregation_REDUCE_PERCENTILE_99
	default:
		return monitoringpb.Aggregation_REDUCE_NONE
	}
}

func numericTypedValue(value *monitoringpb.TypedValue) float64 {
	if value == nil {
		return 0
	}
	switch typed := value.Value.(type) {
	case *monitoringpb.TypedValue_DoubleValue:
		return typed.DoubleValue
	case *monitoringpb.TypedValue_Int64Value:
		return float64(typed.Int64Value)
	case *monitoringpb.TypedValue_BoolValue:
		if typed.BoolValue {
			return 1
		}
	case *monitoringpb.TypedValue_DistributionValue:
		if typed.DistributionValue.GetCount() > 0 {
			return typed.DistributionValue.GetMean()
		}
	}
	return 0
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}

func resourceDisplayName(labels map[string]string) string {
	for _, key := range []string{"service_name", "database_id", "queue_id", "job_id", "model_user_id", "project_id"} {
		if value := labels[key]; value != "" {
			return value
		}
	}
	return "resource"
}

// RealLoggingClient queries Google Cloud Logging.
type RealLoggingClient struct {
	adminClient *logging.Client
}

// NewRealLoggingClient creates a logging admin client.
func NewRealLoggingClient(ctx context.Context, projectID string) (*RealLoggingClient, error) {
	client, err := logging.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create logging client: %w", err)
	}
	return &RealLoggingClient{adminClient: client}, nil
}

func (l *RealLoggingClient) Close() error {
	if l.adminClient != nil {
		return l.adminClient.Close()
	}
	return nil
}

func (l *RealLoggingClient) QueryLogs(ctx context.Context, projectID, severity, filter string, limit int) ([]LogEntry, error) {
	logFilter := fmt.Sprintf(`severity >= %s`, severity)
	if filter != "" {
		logFilter = fmt.Sprintf(`%s AND (%s)`, logFilter, filter)
	}

	it := l.adminClient.Entries(ctx, logging.Filter(logFilter), logging.ProjectIDs([]string{projectID}))
	var entries []LogEntry

	for {
		entry, err := it.Next()
		if errorsIsIteratorDone(err) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error fetching log entries: %w", err)
		}

		payloadStr := ""
		if entry.Payload != nil {
			payloadStr = fmt.Sprintf("%v", entry.Payload)
		}

		entries = append(entries, LogEntry{
			Timestamp: entry.Timestamp.Format(time.RFC3339),
			Severity:  entry.Severity.String(),
			Resource:  entry.Resource.Type,
			Payload:   payloadStr,
		})

		if len(entries) >= limit {
			break
		}
	}

	return entries, nil
}

// RealDigestClient queries pre-computed snapshots from Firestore.
type RealDigestClient struct {
	firestoreClient *firestore.Client
}

// NewRealDigestClient creates a digest client using Firestore.
func NewRealDigestClient(fs *firestore.Client) *RealDigestClient {
	return &RealDigestClient{firestoreClient: fs}
}

func (d *RealDigestClient) GetDigest(ctx context.Context, lookbackHours int) (*DigestSnapshot, error) {
	if d.firestoreClient == nil {
		return nil, fmt.Errorf("firestore client not initialized")
	}

	iter := d.firestoreClient.Collection("infra_snapshots").
		OrderBy("timestamp", firestore.Desc).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if errorsIsIteratorDone(err) {
		// Return an empty snapshot if none exists yet
		return &DigestSnapshot{
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			LookbackHours: lookbackHours,
			Summary:       "Nenhum snapshot anterior encontrado. Monitoramento operando normalmente.",
			Projects:      []ProjectHealth{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error reading snapshot from firestore: %w", err)
	}

	data := doc.Data()
	var timestampStr string
	switch t := data["timestamp"].(type) {
	case time.Time:
		timestampStr = t.UTC().Format(time.RFC3339)
	case string:
		timestampStr = t
	default:
		if t != nil {
			timestampStr = fmt.Sprintf("%v", t)
		} else {
			timestampStr = time.Now().UTC().Format(time.RFC3339)
		}
	}

	lookback := lookbackHours
	if lb, ok := data["lookback_hours"].(int64); ok {
		lookback = int(lb)
	} else if lb, ok := data["lookback_hours"].(int); ok {
		lookback = lb
	}

	summary := ""
	if s, ok := data["summary"].(string); ok {
		summary = s
	}

	var projects []ProjectHealth
	if rawProjects, ok := data["projects"].([]any); ok {
		for _, rp := range rawProjects {
			if pm, ok := rp.(map[string]any); ok {
				ph := ProjectHealth{}
				if pid, ok := pm["project_id"].(string); ok {
					ph.ProjectID = pid
				}
				if st, ok := pm["status"].(string); ok {
					ph.Status = st
				}
				if er, ok := pm["error_rate"].(float64); ok {
					ph.ErrorRate = er
				}
				if aa, ok := pm["active_alerts"].(int64); ok {
					ph.ActiveAlerts = int(aa)
				}
				if sc, ok := pm["service_count"].(int64); ok {
					ph.ServiceCount = int(sc)
				}
				projects = append(projects, ph)
			}
		}
	}

	return &DigestSnapshot{
		Timestamp:     timestampStr,
		LookbackHours: lookback,
		Summary:       summary,
		Projects:      projects,
	}, nil
}

func errorsIsIteratorDone(err error) bool {
	return err == iterator.Done
}

func parseTimeRange(timeRange string) time.Duration {
	tr := strings.TrimSpace(timeRange)
	tr = strings.TrimPrefix(tr, "now-")
	if tr == "" {
		return 1 * time.Hour
	}
	if d, err := time.ParseDuration(tr); err == nil && d > 0 {
		return d
	}
	if strings.HasSuffix(tr, "d") {
		if days, err := strconv.Atoi(strings.TrimSuffix(tr, "d")); err == nil && days > 0 && days <= 30 {
			return time.Duration(days) * 24 * time.Hour
		}
	}
	switch tr {
	case "1h":
		return 1 * time.Hour
	case "2h":
		return 2 * time.Hour
	case "3h":
		return 3 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "24h", "1d":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	default:
		return 1 * time.Hour
	}
}
