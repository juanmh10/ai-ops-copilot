package tools

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

var billingIdentifier = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// RealBillingClient reads the standard Cloud Billing export using ADC.
type RealBillingClient struct {
	client         *bigquery.Client
	billingProject string
	dataset        string
}

func NewRealBillingClient(ctx context.Context, billingProject, dataset string) (*RealBillingClient, error) {
	if !billingIdentifier.MatchString(billingProject) || !billingIdentifier.MatchString(dataset) {
		return nil, fmt.Errorf("invalid billing project or dataset identifier")
	}
	client, err := bigquery.NewClient(ctx, billingProject, option.WithQuotaProject(billingProject))
	if err != nil {
		return nil, fmt.Errorf("create BigQuery billing client: %w", err)
	}
	return &RealBillingClient{client: client, billingProject: billingProject, dataset: dataset}, nil
}

func (b *RealBillingClient) Close() error {
	if b.client == nil {
		return nil
	}
	return b.client.Close()
}

func (b *RealBillingClient) QueryCosts(ctx context.Context, query BillingQuery) (*BillingResult, error) {
	if b.client == nil {
		return nil, fmt.Errorf("BigQuery billing client is not initialized")
	}
	if query.Months < 0 {
		query.Months = 0
	}
	if query.Months > 120 {
		query.Months = 120
	}
	if query.ProjectID != "" && !isAllowedProject(query.ProjectID) {
		return nil, fmt.Errorf("project_id %q is outside the monitored scope", query.ProjectID)
	}

	table := fmt.Sprintf("`%s.%s.gcp_billing_export_v1_*`", b.billingProject, b.dataset)
	accountRows := ""
	if query.ProjectID == "" {
		accountRows = `
UNION ALL
SELECT invoice_month, 'total-monitored' AS project_id, currency,
  SUM(gross_cost) AS gross_cost,
  SUM(credits_applied) AS credits_applied,
  SUM(net_cost) AS net_cost,
  MAX(last_export) AS last_export,
  'monitored' AS scope
FROM grouped
WHERE project_id IN UNNEST(@monitored_projects)
GROUP BY invoice_month, currency
UNION ALL
SELECT invoice_month, 'total-account' AS project_id, currency,
  SUM(gross_cost) AS gross_cost,
  SUM(credits_applied) AS credits_applied,
  SUM(net_cost) AS net_cost,
  MAX(last_export) AS last_export,
  'account' AS scope
FROM grouped
GROUP BY invoice_month, currency`
	}
	sql := fmt.Sprintf(`WITH export AS (
  SELECT invoice.month AS invoice_month, project.id AS project_id, currency, cost,
    COALESCE((SELECT SUM(c.amount) FROM UNNEST(credits) AS c), 0) AS credit_amount,
    export_time
  FROM %s
), bounded AS (
  SELECT * FROM export
  WHERE (@months_back < 0 OR invoice_month >= FORMAT_DATE('%%Y%%m', DATE_SUB(
    PARSE_DATE('%%Y%%m', (SELECT MAX(invoice_month) FROM export)), INTERVAL @months_back MONTH)))
    AND (@project_id = '' OR project_id = @project_id)
), grouped AS (
  SELECT invoice_month, COALESCE(project_id, 'unattributed') AS project_id, currency,
    SUM(cost) AS gross_cost,
    -SUM(credit_amount) AS credits_applied,
    SUM(cost + credit_amount) AS net_cost,
    MAX(export_time) AS last_export
  FROM bounded
  GROUP BY invoice_month, project_id, currency
)
SELECT invoice_month, project_id, currency, gross_cost, credits_applied, net_cost,
  last_export, 'project' AS scope
FROM grouped
WHERE @project_id != '' OR project_id IN UNNEST(@monitored_projects)
%s
ORDER BY invoice_month DESC, scope, net_cost DESC`, table, accountRows)

	monitoredProjects := []string{"enterprise-core-prod", "enterprise-saas-staging", "enterprise-ai-analytics", "enterprise-telemetry-prod", "enterprise-ai-gateway"}
	params := []bigquery.QueryParameter{
		{Name: "months_back", Value: query.Months - 1},
		{Name: "project_id", Value: query.ProjectID},
		{Name: "monitored_projects", Value: monitoredProjects},
	}
	bqQuery := b.client.Query(sql)
	bqQuery.Parameters = params
	bqQuery.MaxBytesBilled = 50_000_000
	it, err := bqQuery.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("query Cloud Billing export: %w", err)
	}
	result := &BillingResult{
		BillingProject: b.billingProject, Dataset: b.dataset, Months: query.Months,
		AllAvailable: query.Months == 0, Lines: make([]BillingLine, 0),
	}
	seenProjects := make(map[string]bool, len(monitoredProjects))
	for {
		var row struct {
			InvoiceMonth   string    `bigquery:"invoice_month"`
			ProjectID      string    `bigquery:"project_id"`
			Currency       string    `bigquery:"currency"`
			GrossCost      float64   `bigquery:"gross_cost"`
			CreditsApplied float64   `bigquery:"credits_applied"`
			NetCost        float64   `bigquery:"net_cost"`
			LastExport     time.Time `bigquery:"last_export"`
			Scope          string    `bigquery:"scope"`
		}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read Cloud Billing export rows: %w", err)
		}
		lastExport := row.LastExport.UTC().Format(time.RFC3339)
		if lastExport > result.LastExport {
			result.LastExport = lastExport
		}
		result.Lines = append(result.Lines, BillingLine{
			InvoiceMonth: row.InvoiceMonth, ProjectID: row.ProjectID, Currency: row.Currency,
			GrossCost: row.GrossCost, CreditsApplied: row.CreditsApplied, NetCost: row.NetCost,
			LastExport: lastExport, Scope: row.Scope,
		})
		if row.Scope == "project" {
			seenProjects[row.ProjectID] = true
		}
	}
	if query.ProjectID == "" && query.Months == 0 {
		for _, projectID := range monitoredProjects {
			if !seenProjects[projectID] {
				result.MissingProjects = append(result.MissingProjects, projectID)
			}
		}
	}
	return result, nil
}
