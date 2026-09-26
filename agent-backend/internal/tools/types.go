package tools

// Tool Names
const (
	ToolGetDashboardMetrics  = "GetDashboardMetrics"
	ToolQueryCloudMonitoring = "QueryCloudMonitoring"
	ToolQueryCloudLogging    = "QueryCloudLogging"
	ToolGetFirestoreDigest   = "GetFirestoreDigest"
	ToolQueryBillingCosts    = "QueryBillingCosts"
)

// DataPoint represents a time-series point.
type DataPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

// MetricsResult represents data retrieved from Grafana.
type MetricsResult struct {
	PanelID   string         `json:"panel_id"`
	Dashboard string         `json:"dashboard_uid,omitempty"`
	TimeRange string         `json:"time_range"`
	Data      map[string]any `json:"data,omitempty"`
	Status    string         `json:"status"`
}

// DashboardQueryContext is request-scoped and is never persisted or sent to the model.
type DashboardQueryContext struct {
	DashboardUID  string            `json:"dashboard_uid,omitempty"`
	TimeFrom      string            `json:"time_from,omitempty"`
	TimeTo        string            `json:"time_to,omitempty"`
	ActiveFilters map[string]string `json:"active_filters,omitempty"`
	Cookie        string            `json:"-"`
	Authorization string            `json:"-"`
}

// BillingQuery selects a bounded invoice-month window and optional project.
type BillingQuery struct {
	ProjectID string `json:"project_id,omitempty"`
	Months    int    `json:"months,omitempty"`
}

type BillingLine struct {
	InvoiceMonth   string  `json:"invoice_month"`
	ProjectID      string  `json:"project_id"`
	Currency       string  `json:"currency"`
	GrossCost      float64 `json:"gross_cost"`
	CreditsApplied float64 `json:"credits_applied"`
	NetCost        float64 `json:"net_cost"`
	LastExport     string  `json:"last_export"`
	Scope          string  `json:"scope"`
}

type BillingResult struct {
	BillingProject  string        `json:"billing_project"`
	Dataset         string        `json:"dataset"`
	Months          int           `json:"months,omitempty"`
	AllAvailable    bool          `json:"all_available_months"`
	Lines           []BillingLine `json:"lines"`
	MissingProjects []string      `json:"missing_projects,omitempty"`
	LastExport      string        `json:"last_export,omitempty"`
}

// MetricSeries represents a single metric series from Cloud Monitoring.
type MetricSeries struct {
	ResourceName   string            `json:"resource_name"`
	ResourceLabels map[string]string `json:"resource_labels,omitempty"`
	MetricLabels   map[string]string `json:"metric_labels,omitempty"`
	Points         []DataPoint       `json:"points"`
}

// MonitoringQuery controls a read-only Cloud Monitoring query. The dashboard
// stays low-cardinality while the agent may request these dimensions on demand.
type MonitoringQuery struct {
	ProjectID        string   `json:"project_id"`
	MetricType       string   `json:"metric_type"`
	TimeRange        string   `json:"time_range"`
	Filter           string   `json:"filter,omitempty"`
	Aligner          string   `json:"aligner,omitempty"`
	Reducer          string   `json:"reducer,omitempty"`
	AlignmentSeconds int      `json:"alignment_seconds,omitempty"`
	GroupBy          []string `json:"group_by,omitempty"`
	SeriesLimit      int      `json:"series_limit,omitempty"`
}

// MonitoringResult represents metrics retrieved from Cloud Monitoring.
type MonitoringResult struct {
	ProjectID  string          `json:"project_id"`
	MetricType string          `json:"metric_type"`
	TimeRange  string          `json:"time_range"`
	Query      MonitoringQuery `json:"query"`
	Series     []MetricSeries  `json:"series"`
	Summary    string          `json:"summary"`
}

// LogEntry represents an individual log entry from Cloud Logging.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Severity  string `json:"severity"`
	Resource  string `json:"resource"`
	Payload   string `json:"payload"`
}

// ProjectHealth represents summary health for one of the 5 projects.
type ProjectHealth struct {
	ProjectID    string  `json:"project_id"`
	Status       string  `json:"status"`
	ErrorRate    float64 `json:"error_rate"`
	ActiveAlerts int     `json:"active_alerts"`
	ServiceCount int     `json:"service_count"`
}

// DigestSnapshot represents a telemetry snapshot saved in Firestore.
type DigestSnapshot struct {
	Timestamp     string          `json:"timestamp"`
	LookbackHours int             `json:"lookback_hours"`
	Summary       string          `json:"summary"`
	Projects      []ProjectHealth `json:"projects"`
}
