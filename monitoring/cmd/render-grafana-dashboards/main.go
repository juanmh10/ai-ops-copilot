// Command render-grafana-dashboards generates the provisioned Grafana dashboard
// suite from a small, reviewable catalog. It performs local file generation only.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	scopingProject = "enterprise-core-prod"
	billingTable   = "`billing_export.gcp_billing_export_v1_*`"
)

var (
	monitoringDS = map[string]any{"type": "stackdriver", "uid": "gcp-monitoring"}
	billingDS    = map[string]any{"type": "grafana-bigquery-datasource", "uid": "gcp-billing"}
)

type projectSpec struct {
	ID          string
	UID         string
	File        string
	Name        string
	Environment string
	Summary     string
	Services    []string
	Extras      string
}

func main() {
	output := flag.String("output", "grafana-provisioning/dashboards/json", "diretório relativo de saída")
	flag.Parse()

	if filepath.IsAbs(*output) {
		fatalf("o diretório de saída deve ser relativo à raiz do repositório")
	}
	if err := os.MkdirAll(*output, 0o755); err != nil {
		fatalf("criando diretório de saída: %v", err)
	}

	dashboards := map[string]map[string]any{
		"unified-cloudrun-dashboard.json": overviewDashboard(),
	}
	for _, spec := range projectSpecs() {
		if spec.ID == "enterprise-core-prod" {
			dashboards[spec.File] = websiteProdDashboard(spec)
		} else if spec.ID == "enterprise-saas-staging" {
			dashboards[spec.File] = condSaasDashboard(spec)
		} else if spec.ID == "enterprise-ai-analytics" {
			dashboards[spec.File] = aiAnalyticsDashboard(spec)
		} else if spec.ID == "enterprise-telemetry-prod" {
			dashboards[spec.File] = telemetriaTrackerDashboard(spec)
		} else {
			dashboards[spec.File] = projectDashboard(spec)
		}
	}
	dashboards["ai-ops-enterprise-ai-gateway.json"] = llmDashboard()

	for name, dashboard := range dashboards {
		data, err := json.MarshalIndent(dashboard, "", "  ")
		if err != nil {
			fatalf("serializando %s: %v", name, err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(filepath.Join(*output, name), data, 0o644); err != nil {
			fatalf("gravando %s: %v", name, err)
		}
	}

	fmt.Printf("generated %d Grafana dashboards in %s\n", len(dashboards), *output)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func projectSpecs() []projectSpec {
	return []projectSpec{
		{
			ID:          "enterprise-core-prod",
			UID:         "aiops-core-prod",
			File:        "ai-ops-enterprise-core-prod.json",
			Name:        "Core Platform & APIs",
			Environment: "production",
			Summary:     "Core production platform and public ingress APIs. Scale down to zero is expected behavior during idle traffic.",
			Services:    []string{"portal-api", "auth-gateway-api"},
			Extras:      "website",
		},
		{
			ID:          "enterprise-saas-staging",
			UID:         "aiops-saas-staging",
			File:        "ai-ops-enterprise-saas-staging.json",
			Name:        "Enterprise SaaS Staging",
			Environment: "staging",
			Summary:     "Staging API and worker, transactional task queue, and Firestore. Focuses on queue depth, error rates, and task latency.",
			Services:    []string{"stg-saas-api", "stg-task-worker"},
			Extras:      "tasks-firestore",
		},
		{
			ID:          "enterprise-ai-analytics",
			UID:         "aiops-ai-analytics",
			File:        "ai-ops-enterprise-ai-analytics.json",
			Name:        "AI Analytics & FinOps",
			Environment: "production",
			Summary:     "API, App Hosting, and billing intelligence. Highlights availability, cloud costs, and BigQuery query health.",
			Services:    []string{"ai-service-api", "ai-portal-frontend"},
			Extras:      "bigquery",
		},
		{
			ID:          "enterprise-telemetry-prod",
			UID:         "aiops-telemetry-prod",
			File:        "ai-ops-enterprise-telemetry-prod.json",
			Name:        "Telemetry Store & Ingest",
			Environment: "production-staging",
			Summary:     "Telemetry ingestion and storage, Firestore Enterprise, and scheduled routines. Cloud Scheduler health is operational priority.",
			Services:    []string{"telemetry-ingest-prod", "telemetry-ingest-staging"},
			Extras:      "scheduler-firestore",
		},
	}
}

func overviewDashboard() map[string]any {
	const tokens = "aiplatform.googleapis.com/publisher/online_serving/token_count"
	const calls = "aiplatform.googleapis.com/publisher/online_serving/model_invocation_count"
	const requests = "run.googleapis.com/request_count"
	project := []string{"resource.label.project_id"}
	model := []string{"resource.label.model_user_id"}
	errors := []string{"metric.label.response_code_class", "=", "5xx"}
	target := func(ref, metric, aligner string, groups, filters []string, alias string) any {
		return overviewTarget(ref, metric, "DELTA", "INT64", aligner, "REDUCE_SUM", groups, filters, alias)
	}
	tokenTypes := func(aligner string) []any {
		return []any{
			target("A", tokens, aligner, nil, []string{"metric.label.type", "=", "input"}, "Entrada"),
			target("B", tokens, aligner, nil, []string{"metric.label.type", "=", "output"}, "Saída"),
		}
	}
	panels := []any{
		textPanel(1, "", 0, 0, 24, 3, "## Seu ecossistema, em uma leitura\n\n5 projetos · Operação, consumo de IA e custos. **Totais seguem o período selecionado; custos seguem a fatura indicada.**"),
		overviewPanel(2, "Requisições · período", "stat", 0, 3, 6, 4, "short", "sum", "blue",
			"Soma das requisições Cloud Run recebidas no período, incluindo respostas com erro.",
			[]any{target("A", requests, "ALIGN_SUM", nil, nil, "Requisições")}),
		overviewPanel(3, "Respostas 5xx · período", "stat", 6, 3, 6, 4, "short", "sum", "orange",
			"Quantidade de respostas com erro de servidor. Ausência de série não comprova zero erros.",
			[]any{target("A", requests, "ALIGN_SUM", nil, errors, "Respostas 5xx")}),
		overviewPanel(4, "Tokens de IA · período", "stat", 12, 3, 6, 4, "short", "sum", "purple",
			"Entrada + saída reportadas pelo Vertex AI nos cinco projetos. Não inclui chamadas diretas à Gemini Developer API.",
			[]any{target("A", tokens, "ALIGN_SUM", nil, nil, "Tokens")}),
		overviewPanel(5, "Chamadas de IA · período", "stat", 18, 3, 6, 4, "short", "sum", "cyan",
			"Invocações Vertex AI no período, incluindo tentativas com erro. Não equivale a conversas ou usuários.",
			[]any{target("A", calls, "ALIGN_SUM", nil, nil, "Chamadas")}),
		overviewRow(20, "01  Operação · onde há tráfego e falhas?", 7),
		overviewPanel(6, "Tráfego por projeto", "timeseries", 0, 8, 12, 8, "reqps", "", "",
			"Requisições por segundo. Cores e nomes dos projetos são consistentes entre painéis.",
			[]any{target("A", requests, "ALIGN_RATE", project, nil, "{{resource.label.project_id}}")}),
		overviewPanel(7, "Erros de servidor por projeto", "timeseries", 12, 8, 12, 8, "reqps", "", "",
			"Respostas HTTP 5xx por segundo. Compare com o tráfego ao lado; não é uma taxa percentual.",
			[]any{target("A", requests, "ALIGN_RATE", project, errors, "{{resource.label.project_id}}")}),
		overviewPanel(8, "Latência p95 · serviço mais lento por projeto", "timeseries", 0, 16, 12, 8, "ms", "", "",
			"Maior p95 entre as séries de serviço/revisão de cada projeto em cada intervalo. Não é o p95 global das requisições; destaca o pior sinal.",
			[]any{overviewTarget("A", "run.googleapis.com/request_latencies", "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_MAX", project, nil, "{{resource.label.project_id}}")}),
		overviewPanel(9, "Instâncias ativas por projeto", "timeseries", 12, 16, 12, 8, "short", "", "",
			"Média de instâncias ativas no intervalo, somada por projeto. Pode ser fracionária. Escala até zero é esperada; dados ausentes não comprovam indisponibilidade.",
			[]any{overviewTarget("A", "run.googleapis.com/container/instance_count", "GAUGE", "INT64", "ALIGN_MEAN", "REDUCE_SUM", project, []string{"metric.label.state", "=", "active"}, "{{resource.label.project_id}}")}),
		overviewRow(21, "02  Inteligência artificial · quanto e quais modelos?", 24),
		overviewPanel(10, "Tokens · entrada e saída", "bargauge", 0, 25, 8, 9, "short", "sum", "",
			"Totais de tokens do período. Entrada inclui o contexto enviado; saída é o conteúdo gerado reportado pelo Vertex AI.",
			tokenTypes("ALIGN_SUM")),
		overviewPanel(11, "Modelos chamados · participação", "piechart", 8, 25, 8, 9, "short", "sum", "",
			"Cada fatia representa a participação de um modelo nas invocações Vertex AI do período. A legenda mostra quantidade e percentual; não representa consumo de tokens.",
			[]any{target("A", calls, "ALIGN_SUM", model, nil, "{{resource.label.model_user_id}}")}),
		overviewPanel(12, "Tokens consumidos por modelo", "bargauge", 16, 25, 8, 9, "short", "sum", "",
			"Entrada + saída por modelo no período. Compare com a participação em chamadas: modelos menos chamados podem consumir mais tokens.",
			[]any{target("A", tokens, "ALIGN_SUM", model, nil, "{{resource.label.model_user_id}}")}),
		overviewPanel(13, "Ritmo de consumo de tokens", "timeseries", 0, 34, 12, 8, "suffix: tokens/s", "", "",
			"Tokens por segundo; a unidade permanece comparável ao mudar o período ou a resolução.",
			tokenTypes("ALIGN_RATE")),
		overviewPanel(14, "Chamadas por modelo ao longo do tempo", "timeseries", 12, 34, 12, 8, "reqps", "", "",
			"Invocações Vertex AI por segundo, separadas por modelo.",
			[]any{target("A", calls, "ALIGN_RATE", model, nil, "{{resource.label.model_user_id}}")}),
		overviewRow(22, "03  Custos · fatura disponível e atualização da fonte", 42),
	}
	summary := bigQueryPanel(15, "Bruto, créditos e líquido · última fatura", "table", 0, 43, 18, 5, "short", overviewBillingSQL("summary"), 1, nil)
	freshness := bigQueryPanel(16, "Defasagem do billing", "stat", 18, 43, 6, 5, "suffix: dias", billingFreshnessSQL(), 1, freshnessThresholds())
	// Give the six-column billing table enough room to stay readable at 1280 px
	// with Grafana's navigation menu open; the monthly chart uses the remaining grid.
	costs := bigQueryPanel(17, "Custo por projeto · fatura mais recente no export", "table", 0, 48, 14, 9, "short", overviewBillingSQL("projects"), 1, nil)
	trend := bigQueryPanel(18, "Custos mensais por fatura · cinco projetos", "barchart", 14, 48, 10, 9, "short", overviewBillingSQL("trend"), 1, nil)
	for _, p := range []map[string]any{summary, freshness, costs, trend} {
		fc := p["fieldConfig"].(map[string]any)
		defaults := fc["defaults"].(map[string]any)
		defaults["noValue"] = "Sem dados"
		defaults["decimals"] = 2
		defaults["custom"] = map[string]any{
			"align": "auto",
			"width": 72,
			"cellOptions": map[string]any{
				"type":     "auto",
				"wrapText": true,
			},
		}
		p["description"] = "Todos os meses presentes no export Standard. Valores na moeda indicada, sem conversão. Créditos são deduzidos do custo bruto. A ausência de linhas aparece como Sem dados e não como custo zero."
		if p["type"] == "table" {
			p["options"] = map[string]any{"showHeader": true, "cellHeight": "md", "footer": map[string]any{"show": false}}
			fc["overrides"] = []any{
				fieldOverride("Escopo", "custom.width", 185),
				fieldOverride("Projeto", "custom.width", 156),
				fieldOverride("Fatura", "custom.width", 96),
				fieldOverride("Fatura", "unit", "string"),
				fieldOverride("Moeda", "unit", "string"),
				fieldOverride("Moeda", "custom.width", 66),
				fieldOverride("Creditos", "displayName", "Créditos"),
				fieldOverride("Liquido", "displayName", "Líquido"),
			}
		}
	}
	costDefaults := costs["fieldConfig"].(map[string]any)["defaults"].(map[string]any)
	costDefaults["noValue"] = "—"
	costs["description"] = "Para cada projeto, mostra a fatura mais recente no export. Quando não existe linha, a coluna Fatura mostra Sem dados e os valores financeiros ficam como —, nunca como zero."
	freshness["description"] = "Dias desde a última atualização do export de custos da conta, medidos por export_time. Acima de 2 dias: atenção; a partir de 7: obsoleto. Sem dados não significa atualizado."
	freshness["targets"].([]any)[0].(map[string]any)["rawSql"] = overviewBillingSQL("freshness")
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["decimals"] = 0
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["displayName"] = "Defasagem"
	trend["description"] = "Todos os meses de fatura presentes no export para os cinco projetos monitorados. Bruto, créditos aplicados e líquido são séries separadas. Cada moeda permanece separada; não há conversão nem previsão."
	trend["options"] = map[string]any{"xField": "Fatura", "orientation": "vertical", "showValue": "never", "groupWidth": 0.7, "barWidth": 0.8, "xTickLabelRotation": -45, "legend": map[string]any{"showLegend": false}, "tooltip": map[string]any{"mode": "single", "sort": "none"}}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["custom"] = map[string]any{"fillOpacity": 85, "lineWidth": 0, "axisLabel": "Valor na moeda indicada"}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["color"] = map[string]any{"mode": "palette-classic"}
	panels = append(panels, summary, freshness, costs, trend)
	note := textPanel(19, "", 0, 57, 24, 3, "**Como interpretar:** Sem dados ≠ zero. Cloud Run pode escalar até zero. IA cobre apenas Vertex AI. Billing é histórico e pode estar atrasado. Abra o projeto para investigar serviços e recursos.")
	note["transparent"] = true
	panels = append(panels, note)
	d := dashboard("Visão Geral — AI-Ops Copilot", "unified-cloudrun-ops", "Operação, consumo Vertex AI e custos dos cinco projetos, com unidades explícitas e ausência de dados visível.", panels, variables("global", "mixed", "all"))
	d["time"] = map[string]any{"from": "now-24h", "to": "now"}
	d["refresh"] = "5m"
	links := []any{}
	for _, spec := range projectSpecs() {
		links = append(links, map[string]any{"title": spec.Name, "type": "link", "url": "/d/" + spec.UID, "includeVars": false, "keepTime": true})
	}
	links = append(links, map[string]any{"title": "Gateway de IA", "type": "link", "url": "/d/aiops-ai-gateway", "keepTime": true})
	d["links"] = links
	return d
}

func overviewRow(id int, title string, y int) map[string]any {
	return map[string]any{"id": id, "title": title, "type": "row", "gridPos": grid(0, y, 24, 1), "collapsed": false, "panels": []any{}}
}

func fieldOverride(name, property string, value any) map[string]any {
	return map[string]any{"matcher": map[string]any{"id": "byName", "options": name}, "properties": []any{map[string]any{"id": property, "value": value}}}
}

// Overview styling is opt-in, so child dashboards remain unchanged.
func overviewPanel(id int, title, kind string, x, y, w, h int, unit, calculation, color, description string, targets []any) map[string]any {
	p := monitoringPanel(id, title, kind, x, y, w, h, unit, targets, nil)
	p["description"] = description
	p["maxDataPoints"] = 1500
	fc := p["fieldConfig"].(map[string]any)
	defaults := fc["defaults"].(map[string]any)
	defaults["noValue"] = "Sem dados"
	defaults["mappings"] = []any{map[string]any{"type": "special", "options": map[string]any{"match": "null", "result": map[string]any{"text": "Sem dados", "color": "gray"}}}}
	defaults["min"] = 0
	defaults["color"] = map[string]any{"mode": "palette-classic"}
	delete(defaults, "displayName")
	defaults["custom"] = map[string]any{
		"drawStyle": "line", "lineInterpolation": "linear", "lineWidth": 2,
		"fillOpacity": 8, "showPoints": "never", "spanNulls": false,
		"axisBorderShow": false, "axisLabel": "", "axisPlacement": "auto",
	}
	overrides := []any{
		fieldOverride("Entrada", "color", map[string]any{"mode": "fixed", "fixedColor": "purple"}),
		fieldOverride("Saída", "color", map[string]any{"mode": "fixed", "fixedColor": "cyan"}),
	}
	colors := []string{"blue", "orange", "purple", "green"}
	for i, spec := range projectSpecs() {
		overrides = append(overrides,
			fieldOverride(spec.ID, "displayName", spec.Name),
			fieldOverride(spec.ID, "color", map[string]any{"mode": "fixed", "fixedColor": colors[i]}))
	}
	fc["overrides"] = overrides
	reduce := map[string]any{"values": false, "calcs": []string{calculation}, "fields": ""}
	switch kind {
	case "stat":
		defaults["decimals"] = 0
		defaults["color"] = map[string]any{"mode": "fixed", "fixedColor": color}
		p["options"] = map[string]any{"reduceOptions": reduce, "orientation": "horizontal", "textMode": "value", "colorMode": "value", "graphMode": "none", "justifyMode": "left", "wideLayout": true}
	case "piechart":
		defaults["decimals"] = 0
		p["options"] = map[string]any{"reduceOptions": reduce, "pieType": "donut", "displayLabels": []string{"percent"}, "legend": map[string]any{"showLegend": true, "displayMode": "table", "placement": "bottom", "values": []string{"value", "percent"}}, "tooltip": map[string]any{"mode": "single", "sort": "desc"}}
	case "bargauge":
		defaults["decimals"] = 0
		// Compute totals before Grafana derives the shared bar scale; otherwise
		// period sums exceed the per-interval maximum and every bar looks full.
		p["transformations"] = []any{map[string]any{"id": "reduce", "options": map[string]any{"reducers": []string{"sum"}, "mode": "reduceFields"}}}
		p["options"] = map[string]any{"reduceOptions": reduce, "orientation": "horizontal", "displayMode": "basic", "showUnfilled": false, "valueMode": "color", "namePlacement": "top", "sizing": "auto", "minVizHeight": 16, "minVizWidth": 0, "maxVizHeight": 60}
		p["options"].(map[string]any)["text"] = map[string]any{"titleSize": 14, "valueSize": 14}
	default:
		p["options"] = map[string]any{
			"legend":  map[string]any{"showLegend": true, "displayMode": "list", "placement": "bottom", "calcs": []string{}},
			"tooltip": map[string]any{"mode": "multi", "sort": "desc"},
		}
	}
	return p
}

func overviewTarget(ref, metric, kind, valueType, aligner, reducer string, groups, filters []string, alias string) map[string]any {
	scope := []string{"resource.label.project_id", "=~", "enterprise-core-prod|enterprise-saas-staging|enterprise-ai-analytics|enterprise-telemetry-prod|enterprise-ai-gateway"}
	if len(filters) > 0 {
		scope = append(append(scope, "AND"), filters...)
	}
	t := monitoringTarget(ref, scopingProject, metric, kind, valueType, aligner, reducer, groups, scope, alias)
	// Grafana 11 migrates legacy metricQuery aliases to the top-level query.
	// Providing only top-level aliasBy with queryType=metrics loses the alias.
	t["metricQuery"].(map[string]any)["aliasBy"] = alias
	return t
}

func overviewBillingSQL(view string) string {
	base := fmt.Sprintf(`WITH billing AS (
  SELECT invoice.month AS invoice_month, project.id AS project_id, currency, cost,
    COALESCE((SELECT SUM(c.amount) FROM UNNEST(credits) c), 0) AS credit_amount
  FROM %s
)
`, billingTable)
	switch view {
	case "freshness":
		return billingFreshnessSQL()
	case "trend":
		return base + `SELECT CONCAT(FORMAT_DATE('%m/%Y', PARSE_DATE('%Y%m', invoice_month)), ' · ', currency) AS Fatura,
  ROUND(SUM(cost), 2) AS Bruto,
  ROUND(-SUM(credit_amount), 2) AS Creditos,
  ROUND(SUM(cost + credit_amount), 2) AS Liquido
FROM billing
WHERE project_id IN ('enterprise-core-prod', 'enterprise-saas-staging', 'enterprise-ai-analytics', 'enterprise-telemetry-prod', 'enterprise-ai-gateway')
GROUP BY invoice_month, currency ORDER BY invoice_month, currency`
	case "projects":
		return base + `, projects AS (
  SELECT 'enterprise-core-prod' AS project_id, 'Core Platform & APIs' AS project_name UNION ALL
  SELECT 'enterprise-saas-staging', 'Enterprise SaaS Staging' UNION ALL
  SELECT 'enterprise-ai-analytics', 'AI Analytics & FinOps' UNION ALL
  SELECT 'enterprise-telemetry-prod', 'Telemetry Store & Ingest' UNION ALL
  SELECT 'enterprise-ai-gateway', 'AI Model Gateway'
), latest AS (
  SELECT project_id, MAX(invoice_month) AS invoice_month
  FROM billing
  WHERE project_id IN ('enterprise-core-prod', 'enterprise-saas-staging', 'enterprise-ai-analytics', 'enterprise-telemetry-prod', 'enterprise-ai-gateway')
  GROUP BY project_id
)
SELECT p.project_name AS Projeto,
  COALESCE(FORMAT_DATE('%m/%Y', PARSE_DATE('%Y%m', latest.invoice_month)), 'Sem dados') AS Fatura,
  COALESCE(b.currency, '—') AS Moeda,
  ROUND(SUM(b.cost), 2) AS Bruto,
  ROUND(-SUM(b.credit_amount), 2) AS Creditos,
  ROUND(SUM(b.cost + b.credit_amount), 2) AS Liquido
FROM projects AS p
LEFT JOIN latest ON latest.project_id = p.project_id
LEFT JOIN billing AS b ON b.project_id = p.project_id AND b.invoice_month = latest.invoice_month
GROUP BY p.project_id, p.project_name, latest.invoice_month, b.currency
ORDER BY p.project_id, b.currency`
	default:
		return base + `, monitored_latest AS (
  SELECT MAX(invoice_month) AS invoice_month FROM billing
  WHERE project_id IN ('enterprise-core-prod', 'enterprise-saas-staging', 'enterprise-ai-analytics', 'enterprise-telemetry-prod', 'enterprise-ai-gateway')
), account_latest AS (
  SELECT MAX(invoice_month) AS invoice_month FROM billing
)
SELECT '5 projetos monitorados' AS Escopo,
  FORMAT_DATE('%m/%Y', PARSE_DATE('%Y%m', invoice_month)) AS Fatura,
  currency AS Moeda, ROUND(SUM(cost), 2) AS Bruto,
  ROUND(-SUM(credit_amount), 2) AS Creditos,
  ROUND(SUM(cost + credit_amount), 2) AS Liquido
FROM billing
WHERE project_id IN ('enterprise-core-prod', 'enterprise-saas-staging', 'enterprise-ai-analytics', 'enterprise-telemetry-prod', 'enterprise-ai-gateway')
  AND invoice_month = (SELECT invoice_month FROM monitored_latest)
GROUP BY invoice_month, currency
UNION ALL
SELECT 'Toda a conta' AS Escopo,
  FORMAT_DATE('%m/%Y', PARSE_DATE('%Y%m', invoice_month)) AS Fatura,
  currency AS Moeda, ROUND(SUM(cost), 2) AS Bruto,
  ROUND(-SUM(credit_amount), 2) AS Creditos,
  ROUND(SUM(cost + credit_amount), 2) AS Liquido
FROM billing
WHERE invoice_month = (SELECT invoice_month FROM account_latest)
GROUP BY invoice_month, currency
ORDER BY Escopo, Moeda`
	}
}

func projectTarget(ref, projectID, metric, kind, valueType, aligner, reducer string, groups, filters []string, alias string) map[string]any {
	allFilters := []string{"resource.label.project_id", "=", projectID}
	if len(filters) > 0 {
		allFilters = append(allFilters, "AND")
		allFilters = append(allFilters, filters...)
	}
	t := monitoringTarget(ref, projectID, metric, kind, valueType, aligner, reducer, groups, allFilters, alias)
	t["metricQuery"].(map[string]any)["aliasBy"] = alias
	return t
}

func projectBillingSQL(projectID, view string) string {
	base := fmt.Sprintf(`WITH billing AS (
  SELECT invoice.month AS invoice_month, project.id AS project_id,
    service.description AS service_name, currency, cost,
    COALESCE((SELECT SUM(c.amount) FROM UNNEST(credits) c), 0) AS credit_amount
  FROM %s
  WHERE project.id = '%s'
), latest AS (SELECT MAX(invoice_month) AS month FROM billing)
`, billingTable, strings.ReplaceAll(projectID, "'", ""))
	switch view {
	case "freshness":
		return billingFreshnessSQL()
	case "trend":
		return base + `SELECT CONCAT(FORMAT_DATE('%m/%Y', PARSE_DATE('%Y%m', invoice_month)), ' · ', currency) AS Fatura,
  ROUND(SUM(cost + credit_amount), 2) AS Liquido
FROM billing GROUP BY invoice_month, currency ORDER BY invoice_month, currency`
	case "services":
		return base + `SELECT FORMAT_DATE('%m/%Y', PARSE_DATE('%Y%m', invoice_month)) AS Fatura,
  COALESCE(service_name, 'Outros') AS Servico,
  currency AS Moeda,
  ROUND(SUM(cost), 2) AS Bruto,
  ROUND(-SUM(credit_amount), 2) AS Creditos,
  ROUND(SUM(cost + credit_amount), 2) AS Liquido
FROM billing WHERE invoice_month = (SELECT month FROM latest)
GROUP BY invoice_month, service_name, currency ORDER BY Liquido DESC`
	default:
		return base + `SELECT FORMAT_DATE('%m/%Y', PARSE_DATE('%Y%m', invoice_month)) AS Fatura,
  currency AS Moeda, ROUND(SUM(cost), 2) AS Bruto,
  ROUND(-SUM(credit_amount), 2) AS Creditos,
  ROUND(SUM(cost + credit_amount), 2) AS Liquido
FROM billing WHERE invoice_month = (SELECT month FROM latest)
GROUP BY invoice_month, currency`
	}
}

func childPanel(id int, title, kind string, x, y, w, h int, unit, calculation, color, description string, targets []any, overrides []any) map[string]any {
	p := monitoringPanel(id, title, kind, x, y, w, h, unit, targets, nil)
	p["description"] = description
	p["maxDataPoints"] = 1500
	fc := p["fieldConfig"].(map[string]any)
	defaults := fc["defaults"].(map[string]any)
	defaults["noValue"] = "Sem dados"
	defaults["mappings"] = []any{map[string]any{"type": "special", "options": map[string]any{"match": "null", "result": map[string]any{"text": "Sem dados", "color": "gray"}}}}
	defaults["min"] = 0
	defaults["color"] = map[string]any{"mode": "palette-classic"}
	delete(defaults, "displayName")
	defaults["custom"] = map[string]any{
		"drawStyle": "line", "lineInterpolation": "linear", "lineWidth": 2,
		"fillOpacity": 8, "showPoints": "never", "spanNulls": false,
		"axisBorderShow": false, "axisLabel": "", "axisPlacement": "auto",
	}
	if len(overrides) > 0 {
		fc["overrides"] = overrides
	}
	reduce := map[string]any{"values": false, "calcs": []string{calculation}, "fields": ""}
	switch kind {
	case "stat":
		defaults["decimals"] = 0
		if color != "" {
			defaults["color"] = map[string]any{"mode": "fixed", "fixedColor": color}
		}
		p["options"] = map[string]any{"reduceOptions": reduce, "orientation": "horizontal", "textMode": "value", "colorMode": "value", "graphMode": "none", "justifyMode": "left", "wideLayout": true}
	case "piechart":
		defaults["decimals"] = 0
		p["options"] = map[string]any{"reduceOptions": reduce, "pieType": "donut", "displayLabels": []string{"percent"}, "legend": map[string]any{"showLegend": true, "displayMode": "table", "placement": "bottom", "values": []string{"value", "percent"}}, "tooltip": map[string]any{"mode": "single", "sort": "desc"}}
	case "bargauge":
		defaults["decimals"] = 0
		p["transformations"] = []any{map[string]any{"id": "reduce", "options": map[string]any{"reducers": []string{"sum"}, "mode": "reduceFields"}}}
		p["options"] = map[string]any{"reduceOptions": reduce, "orientation": "horizontal", "displayMode": "basic", "showUnfilled": false, "valueMode": "color", "namePlacement": "top", "sizing": "auto", "minVizHeight": 16, "minVizWidth": 0, "maxVizHeight": 60}
		p["options"].(map[string]any)["text"] = map[string]any{"titleSize": 14, "valueSize": 14}
	default:
		p["options"] = map[string]any{
			"legend":  map[string]any{"showLegend": true, "displayMode": "list", "placement": "bottom", "calcs": []string{}},
			"tooltip": map[string]any{"mode": "multi", "sort": "desc"},
		}
	}
	return p
}

func childBigQueryPanel(id int, title, kind string, x, y, w, h int, unit, sql string, format int, thresholds map[string]any, description string) map[string]any {
	p := bigQueryPanel(id, title, kind, x, y, w, h, unit, sql, format, thresholds)
	p["description"] = description
	fc := p["fieldConfig"].(map[string]any)
	defaults := fc["defaults"].(map[string]any)
	defaults["noValue"] = "Sem dados"
	defaults["decimals"] = 2
	defaults["custom"] = map[string]any{"align": "auto", "width": 72}
	return p
}

func projectNavLinks(currentUID string) []any {
	links := []any{
		map[string]any{"title": "Visão Geral", "type": "link", "url": "/d/unified-cloudrun-ops", "keepTime": true},
	}
	for _, spec := range projectSpecs() {
		if spec.UID == currentUID {
			continue
		}
		links = append(links, map[string]any{"title": spec.Name, "type": "link", "url": "/d/" + spec.UID, "keepTime": true})
	}
	if currentUID != "aiops-ai-gateway" {
		links = append(links, map[string]any{"title": "Gateway de IA", "type": "link", "url": "/d/aiops-ai-gateway", "keepTime": true})
	}
	return links
}

func websiteProdDashboard(spec projectSpec) map[string]any {
	const requests = "run.googleapis.com/request_count"
	const latencies = "run.googleapis.com/request_latencies"
	const instances = "run.googleapis.com/container/instance_count"
	const cpu = "run.googleapis.com/container/cpu/utilizations"
	const mem = "run.googleapis.com/container/memory/utilizations"
	service := []string{"resource.label.service_name"}
	errors := []string{"metric.label.response_code_class", "=", "5xx"}
	active := []string{"metric.label.state", "=", "active"}

	target := func(ref, metric, aligner string, groups, filters []string, alias string) any {
		return projectTarget(ref, spec.ID, metric, "DELTA", "INT64", aligner, "REDUCE_SUM", groups, filters, alias)
	}

	serviceOverrides := []any{
		fieldOverride("portal-api", "displayName", "portal-api"),
		fieldOverride("portal-api", "color", map[string]any{"mode": "fixed", "fixedColor": "blue"}),
		fieldOverride("auth-gateway-api", "displayName", "auth-gateway-api"),
		fieldOverride("auth-gateway-api", "color", map[string]any{"mode": "fixed", "fixedColor": "cyan"}),
	}
	errOverrides := []any{
		fieldOverride("portal-api 5xx", "displayName", "portal-api 5xx"),
		fieldOverride("portal-api 5xx", "color", map[string]any{"mode": "fixed", "fixedColor": "orange"}),
		fieldOverride("auth-gateway-api 5xx", "displayName", "auth-gateway-api 5xx"),
		fieldOverride("auth-gateway-api 5xx", "color", map[string]any{"mode": "fixed", "fixedColor": "red"}),
	}
	cpuOverrides := []any{
		fieldOverride("portal-api CPU", "displayName", "portal-api"),
		fieldOverride("portal-api CPU", "color", map[string]any{"mode": "fixed", "fixedColor": "blue"}),
		fieldOverride("auth-gateway-api CPU", "displayName", "auth-gateway-api"),
		fieldOverride("auth-gateway-api CPU", "color", map[string]any{"mode": "fixed", "fixedColor": "cyan"}),
	}
	memOverrides := []any{
		fieldOverride("portal-api Memória", "displayName", "portal-api"),
		fieldOverride("portal-api Memória", "color", map[string]any{"mode": "fixed", "fixedColor": "purple"}),
		fieldOverride("auth-gateway-api Memória", "displayName", "auth-gateway-api"),
		fieldOverride("auth-gateway-api Memória", "color", map[string]any{"mode": "fixed", "fixedColor": "green"}),
	}

	panels := []any{
		textPanel(1, "", 0, 0, 24, 3, "## Website & APIs Core, em uma leitura\n\nProdução · Tráfego, falhas e desempenho das APIs. **Totais seguem o período selecionado; custos seguem a fatura indicada.**"),
		childPanel(2, "Requisições · período", "stat", 0, 3, 6, 4, "short", "sum", "blue",
			"Soma das requisições Cloud Run recebidas no período para portal-api e auth-gateway-api, incluindo erros.",
			[]any{target("A", requests, "ALIGN_SUM", nil, nil, "Requisições")}, nil),
		childPanel(3, "Respostas 5xx", "stat", 6, 3, 6, 4, "short", "sum", "orange",
			"Quantidade de respostas com erro de servidor (HTTP 5xx). Ausência de série não comprova zero erros.",
			[]any{target("A", requests, "ALIGN_SUM", nil, errors, "Respostas 5xx")}, nil),
		childPanel(4, "Pior latência p95", "stat", 12, 3, 6, 4, "ms", "lastNotNull", "purple",
			"Maior latência p95 entre os serviços do projeto no intervalo mais recente. Destaca o pior sinal operacional.",
			[]any{projectTarget("A", spec.ID, latencies, "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_MAX", nil, nil, "Pior p95")}, nil),
		childPanel(5, "Instâncias ativas", "stat", 18, 3, 6, 4, "short", "lastNotNull", "cyan",
			"Média de instâncias ativas no intervalo mais recente, somada entre os serviços. Escala até zero é esperada quando não há tráfego.",
			[]any{projectTarget("A", spec.ID, instances, "GAUGE", "INT64", "ALIGN_MEAN", "REDUCE_SUM", nil, active, "Ativas")}, nil),

		overviewRow(20, "01  Operação · onde há tráfego e falhas?", 7),
		childPanel(6, "Tráfego por serviço", "timeseries", 0, 8, 12, 8, "reqps", "", "",
			"Requisições por segundo por serviço (portal-api e auth-gateway-api).",
			[]any{target("A", requests, "ALIGN_RATE", service, nil, "{{resource.label.service_name}}")}, serviceOverrides),
		childPanel(7, "Erros de servidor por serviço", "timeseries", 12, 8, 12, 8, "reqps", "", "",
			"Respostas HTTP 5xx por segundo. Compare com o tráfego ao lado; não é uma taxa percentual.",
			[]any{target("A", requests, "ALIGN_RATE", service, errors, "{{resource.label.service_name}} 5xx")}, errOverrides),
		childPanel(8, "Latência p95 por serviço", "timeseries", 0, 16, 12, 8, "ms", "", "",
			"Latência p95 em milissegundos por serviço ao longo do tempo.",
			[]any{projectTarget("A", spec.ID, latencies, "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_MAX", service, nil, "{{resource.label.service_name}}")}, serviceOverrides),
		childPanel(9, "Instâncias ativas por serviço", "timeseries", 12, 16, 12, 8, "short", "", "",
			"Média de instâncias ativas no intervalo por serviço. Escala até zero é esperada quando o serviço está inativo.",
			[]any{projectTarget("A", spec.ID, instances, "GAUGE", "INT64", "ALIGN_MEAN", "REDUCE_SUM", service, active, "{{resource.label.service_name}}")}, serviceOverrides),

		overviewRow(21, "02  Recursos · há pressão de CPU ou memória?", 24),
		childPanel(10, "Utilização de CPU p95 por serviço", "timeseries", 0, 25, 12, 8, "percentunit", "", "",
			"Percentil 95 de utilização de CPU alocada por serviço (0.0 a 1.0). Valores acima de 0.8 indicam necessidade de ajuste de capacidade.",
			[]any{projectTarget("A", spec.ID, cpu, "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_MAX", service, nil, "{{resource.label.service_name}} CPU")}, cpuOverrides),
		childPanel(11, "Utilização de memória p95 por serviço", "timeseries", 12, 25, 12, 8, "percentunit", "", "",
			"Percentil 95 de utilização de memória alocada por serviço (0.0 a 1.0). Valores próximos a 1.0 alertam risco de esgotamento de memória (OOM).",
			[]any{projectTarget("A", spec.ID, mem, "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_MAX", service, nil, "{{resource.label.service_name}} Memória")}, memOverrides),

		overviewRow(22, "03  Custos · fatura disponível e atualização da fonte", 33),
	}

	summary := childBigQueryPanel(12, "Resumo da última fatura disponível", "table", 0, 34, 18, 5, "short", projectBillingSQL(spec.ID, "summary"), 1, nil,
		"Custos brutos, créditos aplicados e valor líquido da fatura mais recente para o projeto Website & APIs Core.")
	freshness := childBigQueryPanel(13, "Defasagem do billing", "stat", 18, 34, 6, 5, "suffix: dias", projectBillingSQL(spec.ID, "freshness"), 1, freshnessThresholds(),
		"Dias desde a última atualização do export de custos da conta, medidos por export_time. Acima de 2 dias: atenção; a partir de 7: obsoleto.")
	servicesCost := childBigQueryPanel(14, "Custo por serviço · última fatura", "table", 0, 39, 12, 9, "short", projectBillingSQL(spec.ID, "services"), 1, nil,
		"Detalhamento de custo por serviço na última fatura disponível do projeto.")
	trend := childBigQueryPanel(15, "Custo líquido por mês de fatura", "barchart", 12, 39, 12, 9, "short", projectBillingSQL(spec.ID, "trend"), 1, nil,
		"Até 12 meses de fatura a partir da última fatura disponível. Valores na moeda indicada, sem conversão.")

	for _, p := range []map[string]any{summary, servicesCost} {
		p["options"] = map[string]any{"showHeader": true, "cellHeight": "md", "footer": map[string]any{"show": false}}
		fc := p["fieldConfig"].(map[string]any)
		fc["overrides"] = []any{
			fieldOverride("Servico", "custom.width", 210),
			fieldOverride("Fatura", "unit", "string"),
			fieldOverride("Moeda", "unit", "string"),
			fieldOverride("Moeda", "custom.width", 50),
			fieldOverride("Bruto", "custom.width", 60),
			fieldOverride("Creditos", "custom.width", 68),
			fieldOverride("Creditos", "displayName", "Créditos"),
			fieldOverride("Liquido", "custom.width", 68),
			fieldOverride("Liquido", "displayName", "Líquido"),
		}
	}
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["decimals"] = 0
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["displayName"] = "Defasagem"
	trend["options"] = map[string]any{"xField": "Fatura", "orientation": "vertical", "showValue": "never", "groupWidth": 0.7, "barWidth": 0.8, "xTickLabelRotation": -45, "legend": map[string]any{"showLegend": false}, "tooltip": map[string]any{"mode": "single", "sort": "none"}}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["custom"] = map[string]any{"fillOpacity": 85, "lineWidth": 0, "axisLabel": "Valor na moeda indicada"}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["color"] = map[string]any{"mode": "palette-classic"}

	panels = append(panels, summary, freshness, servicesCost, trend)
	note := textPanel(19, "", 0, 48, 24, 3, "**Como interpretar:** Sem dados ≠ zero. Cloud Run escala até zero quando inativo. Custos são históricos do BigQuery export e podem apresentar defasagem de dias. P95 de CPU/memória indica pressão de pico nas instâncias ativas.")
	note["transparent"] = true
	panels = append(panels, note)

	serviceList := strings.Join(spec.Services, ", ")
	d := dashboard("Projeto — "+spec.Name, spec.UID, spec.Summary, panels, variables(spec.ID, spec.Environment, serviceList))
	d["time"] = map[string]any{"from": "now-24h", "to": "now"}
	d["refresh"] = "5m"
	d["links"] = projectNavLinks(spec.UID)
	return d
}

func condSaasDashboard(spec projectSpec) map[string]any {
	const requests = "run.googleapis.com/request_count"
	const latencies = "run.googleapis.com/request_latencies"
	const instances = "run.googleapis.com/container/instance_count"
	service := []string{"resource.label.service_name"}
	errors := []string{"metric.label.response_code_class", "=", "5xx"}
	active := []string{"metric.label.state", "=", "active"}

	target := func(ref, metric, aligner string, groups, filters []string, alias string) any {
		return projectTarget(ref, spec.ID, metric, "DELTA", "INT64", aligner, "REDUCE_SUM", groups, filters, alias)
	}

	serviceOverrides := []any{
		fieldOverride("stg-saas-api", "displayName", "stg-saas-api"),
		fieldOverride("stg-saas-api", "color", map[string]any{"mode": "fixed", "fixedColor": "blue"}),
		fieldOverride("stg-task-worker", "displayName", "stg-task-worker"),
		fieldOverride("stg-task-worker", "color", map[string]any{"mode": "fixed", "fixedColor": "orange"}),
	}
	errOverrides := []any{
		fieldOverride("stg-saas-api 5xx", "displayName", "stg-saas-api 5xx"),
		fieldOverride("stg-saas-api 5xx", "color", map[string]any{"mode": "fixed", "fixedColor": "orange"}),
		fieldOverride("stg-task-worker 5xx", "displayName", "stg-task-worker 5xx"),
		fieldOverride("stg-task-worker 5xx", "color", map[string]any{"mode": "fixed", "fixedColor": "red"}),
	}

	panels := []any{
		textPanel(1, "", 0, 0, 24, 3, "## Enterprise SaaS Staging at a Glance\n\nStaging · API, task processing, and Firestore. **Totals reflect selected period; costs reflect indicated invoice.**"),
		childPanel(2, "Requisições · período", "stat", 0, 3, 6, 4, "short", "sum", "blue",
			"Soma das requisições Cloud Run no período para stg-saas-api e stg-task-worker, incluindo erros.",
			[]any{target("A", requests, "ALIGN_SUM", nil, nil, "Requisições")}, nil),
		childPanel(3, "Respostas 5xx", "stat", 6, 3, 6, 4, "short", "sum", "orange",
			"Respostas com código 5xx no período. Homologação inativa não comprova zero erros.",
			[]any{target("A", requests, "ALIGN_SUM", nil, errors, "Respostas 5xx")}, nil),
		childPanel(4, "Tarefas aguardando", "stat", 12, 3, 6, 4, "short", "lastNotNull", "purple",
			"Estoque atual de tarefas aguardando processamento na fila Cloud Tasks (queue depth). Ausência de dados indica fila ociosa.",
			[]any{projectTarget("A", spec.ID, "cloudtasks.googleapis.com/queue/depth", "GAUGE", "INT64", "ALIGN_MAX", "REDUCE_MAX", nil, nil, "Aguardando")}, nil),
		childPanel(5, "Pior latência p95", "stat", 18, 3, 6, 4, "ms", "lastNotNull", "cyan",
			"Pior latência p95 registrada entre os serviços da API no intervalo mais recente.",
			[]any{projectTarget("A", spec.ID, latencies, "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_MAX", nil, nil, "Pior p95")}, nil),

		overviewRow(20, "01  Operação · API e serviços", 7),
		childPanel(6, "Tráfego por serviço", "timeseries", 0, 8, 12, 8, "reqps", "", "",
			"Requisições por segundo por serviço (stg-saas-api e stg-task-worker).",
			[]any{target("A", requests, "ALIGN_RATE", service, nil, "{{resource.label.service_name}}")}, serviceOverrides),
		childPanel(7, "Erros de servidor por serviço", "timeseries", 12, 8, 12, 8, "reqps", "", "",
			"Respostas HTTP 5xx por segundo por serviço.",
			[]any{target("A", requests, "ALIGN_RATE", service, errors, "{{resource.label.service_name}} 5xx")}, errOverrides),
		childPanel(8, "Latência p95 por serviço", "timeseries", 0, 16, 12, 8, "ms", "", "",
			"Latência p95 em milissegundos por serviço ao longo do tempo.",
			[]any{projectTarget("A", spec.ID, latencies, "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_MAX", service, nil, "{{resource.label.service_name}} p95")}, serviceOverrides),
		childPanel(9, "Instâncias ativas por serviço", "timeseries", 12, 16, 12, 8, "short", "", "",
			"Média de instâncias ativas no intervalo. Escala a zero é esperada em ambiente de homologação.",
			[]any{projectTarget("A", spec.ID, instances, "GAUGE", "INT64", "ALIGN_MEAN", "REDUCE_SUM", service, active, "{{resource.label.service_name}}")}, serviceOverrides),

		overviewRow(21, "02  Processamento · a fila está avançando?", 24),
		childPanel(10, "Fila · tarefas aguardando", "timeseries", 0, 25, 8, 9, "short", "", "",
			"Profundidade da fila Cloud Tasks (queue depth). Mede estoque de trabalho pendente no worker.",
			[]any{projectTarget("A", spec.ID, "cloudtasks.googleapis.com/queue/depth", "GAUGE", "INT64", "ALIGN_MAX", "REDUCE_MAX", []string{"resource.label.queue_id"}, nil, "{{resource.label.queue_id}}")}, nil),
		childPanel(11, "Fila · tentativas por resposta", "timeseries", 8, 25, 8, 9, "ops", "", "",
			"Taxa de tentativas de execução de tarefas por código de resposta. Tentativas com erro geram retries.",
			[]any{projectTarget("A", spec.ID, "cloudtasks.googleapis.com/queue/task_attempt_count", "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", []string{"metric.label.response_code"}, nil, "{{metric.label.response_code}}")}, nil),
		childPanel(12, "Firestore · latência p95 por método", "timeseries", 16, 25, 8, 9, "s", "", "",
			"Latência p95 das chamadas à API do Firestore (leitura/escrita/commit).",
			[]any{projectTarget("A", spec.ID, "firestore.googleapis.com/api/request_latencies", "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_MAX", []string{"metric.label.api_method"}, nil, "{{metric.label.api_method}}")}, nil),

		overviewRow(22, "03  Custos · fatura disponível e atualização da fonte", 34),
	}

	summary := childBigQueryPanel(13, "Resumo da última fatura disponível", "table", 0, 35, 18, 5, "short", projectBillingSQL(spec.ID, "summary"), 1, nil,
		"Gross costs, applied credits, and net amount for the most recent invoice for Enterprise SaaS Staging.")
	freshness := childBigQueryPanel(14, "Defasagem do billing", "stat", 18, 35, 6, 5, "suffix: dias", projectBillingSQL(spec.ID, "freshness"), 1, freshnessThresholds(),
		"Dias desde a última atualização do export de custos da conta, medidos por export_time. Acima de 2 dias: atenção; a partir de 7: obsoleto.")
	servicesCost := childBigQueryPanel(15, "Custo por serviço · última fatura", "table", 0, 40, 12, 9, "short", projectBillingSQL(spec.ID, "services"), 1, nil,
		"Detalhamento de custo por serviço na última fatura disponível do projeto.")
	trend := childBigQueryPanel(16, "Custo líquido por mês de fatura", "barchart", 12, 40, 12, 9, "short", projectBillingSQL(spec.ID, "trend"), 1, nil,
		"Até 12 meses de fatura a partir da última fatura disponível. Valores na moeda indicada, sem conversão.")

	for _, p := range []map[string]any{summary, servicesCost} {
		p["options"] = map[string]any{"showHeader": true, "cellHeight": "md", "footer": map[string]any{"show": false}}
		fc := p["fieldConfig"].(map[string]any)
		fc["overrides"] = []any{
			fieldOverride("Servico", "custom.width", 210),
			fieldOverride("Fatura", "unit", "string"),
			fieldOverride("Moeda", "unit", "string"),
			fieldOverride("Moeda", "custom.width", 50),
			fieldOverride("Bruto", "custom.width", 60),
			fieldOverride("Creditos", "custom.width", 68),
			fieldOverride("Creditos", "displayName", "Créditos"),
			fieldOverride("Liquido", "custom.width", 68),
			fieldOverride("Liquido", "displayName", "Líquido"),
		}
	}
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["decimals"] = 0
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["displayName"] = "Defasagem"
	trend["options"] = map[string]any{"xField": "Fatura", "orientation": "vertical", "showValue": "never", "groupWidth": 0.7, "barWidth": 0.8, "xTickLabelRotation": -45, "legend": map[string]any{"showLegend": false}, "tooltip": map[string]any{"mode": "single", "sort": "none"}}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["custom"] = map[string]any{"fillOpacity": 85, "lineWidth": 0, "axisLabel": "Valor na moeda indicada"}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["color"] = map[string]any{"mode": "palette-classic"}

	panels = append(panels, summary, freshness, servicesCost, trend)
	note := textPanel(19, "", 0, 49, 24, 3, "**Como interpretar:** Sem dados ≠ zero. Ambiente de homologação pode ficar completamente ocioso sem indicar falha. Profundidade da fila mede estoque pendente, não soma de tarefas processadas. Billing segue o export BigQuery histórico.")
	note["transparent"] = true
	panels = append(panels, note)

	serviceList := strings.Join(spec.Services, ", ")
	d := dashboard("Projeto — "+spec.Name, spec.UID, spec.Summary, panels, variables(spec.ID, spec.Environment, serviceList))
	d["time"] = map[string]any{"from": "now-24h", "to": "now"}
	d["refresh"] = "5m"
	d["links"] = projectNavLinks(spec.UID)
	return d
}

func aiAnalyticsDashboard(spec projectSpec) map[string]any {
	const requests = "run.googleapis.com/request_count"
	const latencies = "run.googleapis.com/request_latencies"
	const instances = "run.googleapis.com/container/instance_count"
	const bqExecution = "bigquery.googleapis.com/query/execution_count"
	const bqBytes = "bigquery.googleapis.com/query/scanned_bytes_billed"
	service := []string{"resource.label.service_name"}
	priority := []string{"metric.label.priority"}
	errors := []string{"metric.label.response_code_class", "=", "5xx"}
	active := []string{"metric.label.state", "=", "active"}

	target := func(ref, metric, aligner string, groups, filters []string, alias string) any {
		return projectTarget(ref, spec.ID, metric, "DELTA", "INT64", aligner, "REDUCE_SUM", groups, filters, alias)
	}

	serviceOverrides := []any{
		fieldOverride("ai-service-api", "displayName", "ai-service-api"),
		fieldOverride("ai-service-api", "color", map[string]any{"mode": "fixed", "fixedColor": "blue"}),
		fieldOverride("ai-portal-frontend", "displayName", "ai-portal-frontend"),
		fieldOverride("ai-portal-frontend", "color", map[string]any{"mode": "fixed", "fixedColor": "cyan"}),
	}
	errOverrides := []any{
		fieldOverride("ai-service-api 5xx", "displayName", "ai-service-api 5xx"),
		fieldOverride("ai-service-api 5xx", "color", map[string]any{"mode": "fixed", "fixedColor": "orange"}),
		fieldOverride("ai-portal-frontend 5xx", "displayName", "ai-portal-frontend 5xx"),
		fieldOverride("ai-portal-frontend 5xx", "color", map[string]any{"mode": "fixed", "fixedColor": "red"}),
	}
	priorityOverrides := []any{
		fieldOverride("interactive", "displayName", "Interativa (interactive)"),
		fieldOverride("interactive", "color", map[string]any{"mode": "fixed", "fixedColor": "blue"}),
		fieldOverride("batch", "displayName", "Em lote (batch)"),
		fieldOverride("batch", "color", map[string]any{"mode": "fixed", "fixedColor": "orange"}),
	}

	panels := []any{
		textPanel(1, "", 0, 0, 24, 3, "## AI Analytics & FinOps at a Glance\n\nProduction · Service API, data processing, and billing. **Totals reflect selected period; costs reflect indicated invoice.**"),
		childPanel(2, "Requisições · período", "stat", 0, 3, 6, 4, "short", "sum", "blue",
			"Soma das requisições Cloud Run recebidas no período para ai-service-api e ai-portal-frontend, incluindo erros.",
			[]any{target("A", requests, "ALIGN_SUM", nil, nil, "Requisições")}, nil),
		childPanel(3, "Respostas 5xx", "stat", 6, 3, 6, 4, "short", "sum", "orange",
			"Quantidade de respostas com erro de servidor (HTTP 5xx). Ausência de série não comprova zero erros.",
			[]any{target("A", requests, "ALIGN_SUM", nil, errors, "Respostas 5xx")}, nil),
		childPanel(4, "Consultas BigQuery", "stat", 12, 3, 6, 4, "short", "sum", "purple",
			"Soma das consultas BigQuery executadas no projeto durante o período selecionado.",
			[]any{projectTarget("A", spec.ID, bqExecution, "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", nil, nil, "Consultas")}, nil),
		childPanel(5, "Bytes faturados", "stat", 18, 3, 6, 4, "decbytes", "sum", "cyan",
			"Total de bytes escaneados faturados por consultas BigQuery no período (scanned_bytes_billed). Não confundir com bytes em armazenamento.",
			[]any{projectTarget("A", spec.ID, bqBytes, "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", nil, nil, "Bytes faturados")}, nil),

		overviewRow(20, "01  Operação · aplicação e serviços", 7),
		childPanel(6, "Tráfego por serviço", "timeseries", 0, 8, 12, 8, "reqps", "", "",
			"Requisições por segundo por serviço (ai-service-api e ai-portal-frontend).",
			[]any{target("A", requests, "ALIGN_RATE", service, nil, "{{resource.label.service_name}}")}, serviceOverrides),
		childPanel(7, "Erros de servidor por serviço", "timeseries", 12, 8, 12, 8, "reqps", "", "",
			"Respostas HTTP 5xx por segundo por serviço.",
			[]any{target("A", requests, "ALIGN_RATE", service, errors, "{{resource.label.service_name}} 5xx")}, errOverrides),
		childPanel(8, "Latência p95 por serviço", "timeseries", 0, 16, 12, 8, "ms", "", "",
			"Latência p95 em milissegundos por serviço ao longo do tempo.",
			[]any{projectTarget("A", spec.ID, latencies, "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_MAX", service, nil, "{{resource.label.service_name}} p95")}, serviceOverrides),
		childPanel(9, "Instâncias ativas por serviço", "timeseries", 12, 16, 12, 8, "short", "", "",
			"Média de instâncias ativas no intervalo. Escala a zero é esperada quando o serviço está inativo.",
			[]any{projectTarget("A", spec.ID, instances, "GAUGE", "INT64", "ALIGN_MEAN", "REDUCE_SUM", service, active, "{{resource.label.service_name}}")}, serviceOverrides),

		overviewRow(21, "02  Dados · quanto estamos processando?", 24),
		childPanel(10, "BigQuery · ritmo de consultas executadas", "timeseries", 0, 25, 12, 8, "ops", "", "",
			"Consultas executadas por segundo agrupadas por prioridade (interactive ou batch).",
			[]any{projectTarget("A", spec.ID, bqExecution, "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", priority, nil, "{{metric.label.priority}}")}, priorityOverrides),
		childPanel(11, "BigQuery · volume escaneado por segundo", "timeseries", 12, 25, 12, 8, "Bps", "", "",
			"Taxa de bytes escaneados faturados por segundo (scanned_bytes_billed) por prioridade. Mede pressão e volume analítico.",
			[]any{projectTarget("A", spec.ID, bqBytes, "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", priority, nil, "{{metric.label.priority}}")}, priorityOverrides),

		overviewRow(22, "03  Custos · fatura disponível e atualização da fonte", 33),
	}

	summary := childBigQueryPanel(12, "Resumo da última fatura disponível", "table", 0, 34, 18, 5, "short", projectBillingSQL(spec.ID, "summary"), 1, nil,
		"Gross costs, applied credits, and net amount for the most recent invoice for AI Analytics & FinOps.")
	freshness := childBigQueryPanel(13, "Defasagem do billing", "stat", 18, 34, 6, 5, "suffix: dias", projectBillingSQL(spec.ID, "freshness"), 1, freshnessThresholds(),
		"Dias desde a última atualização do export de custos da conta, medidos por export_time. Acima de 2 dias: atenção; a partir de 7: obsoleto.")
	servicesCost := childBigQueryPanel(14, "Custo por serviço · última fatura", "table", 0, 39, 12, 9, "short", projectBillingSQL(spec.ID, "services"), 1, nil,
		"Detalhamento de custo por serviço na última fatura disponível do projeto.")
	trend := childBigQueryPanel(15, "Custo líquido por mês de fatura", "barchart", 12, 39, 12, 9, "short", projectBillingSQL(spec.ID, "trend"), 1, nil,
		"Até 12 meses de fatura a partir da última fatura disponível. Valores na moeda indicada, sem conversão.")

	for _, p := range []map[string]any{summary, servicesCost} {
		p["options"] = map[string]any{"showHeader": true, "cellHeight": "md", "footer": map[string]any{"show": false}}
		fc := p["fieldConfig"].(map[string]any)
		fc["overrides"] = []any{
			fieldOverride("Servico", "custom.width", 210),
			fieldOverride("Fatura", "unit", "string"),
			fieldOverride("Moeda", "unit", "string"),
			fieldOverride("Moeda", "custom.width", 50),
			fieldOverride("Bruto", "custom.width", 60),
			fieldOverride("Creditos", "custom.width", 68),
			fieldOverride("Creditos", "displayName", "Créditos"),
			fieldOverride("Liquido", "custom.width", 68),
			fieldOverride("Liquido", "displayName", "Líquido"),
		}
	}
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["decimals"] = 0
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["displayName"] = "Defasagem"
	trend["options"] = map[string]any{"xField": "Fatura", "orientation": "vertical", "showValue": "never", "groupWidth": 0.7, "barWidth": 0.8, "xTickLabelRotation": -45, "legend": map[string]any{"showLegend": false}, "tooltip": map[string]any{"mode": "single", "sort": "none"}}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["custom"] = map[string]any{"fillOpacity": 85, "lineWidth": 0, "axisLabel": "Valor na moeda indicada"}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["color"] = map[string]any{"mode": "palette-classic"}

	panels = append(panels, summary, freshness, servicesCost, trend)
	note := textPanel(19, "", 0, 48, 24, 3, "**Como interpretar:** Sem dados ≠ zero. Cloud Run escala até zero quando inativo. Consultas BigQuery refletem execuções e bytes escaneados faturados (scanned_bytes_billed). Custos são históricos do BigQuery export e podem apresentar defasagem de dias.")
	note["transparent"] = true
	panels = append(panels, note)

	serviceList := strings.Join(spec.Services, ", ")
	d := dashboard("Projeto — "+spec.Name, spec.UID, spec.Summary, panels, variables(spec.ID, spec.Environment, serviceList))
	d["time"] = map[string]any{"from": "now-24h", "to": "now"}
	d["refresh"] = "5m"
	d["links"] = projectNavLinks(spec.UID)
	return d
}

func telemetriaTrackerDashboard(spec projectSpec) map[string]any {
	const requests = "run.googleapis.com/request_count"
	const latencies = "run.googleapis.com/request_latencies"
	const instances = "run.googleapis.com/container/instance_count"
	const schedulerLogs = "logging.googleapis.com/log_entry_count"
	const firestoreRead = "firestore.googleapis.com/document/read_ops_count"
	const firestoreWrite = "firestore.googleapis.com/document/write_ops_count"
	const firestoreStorage = "firestore.googleapis.com/storage/data_and_index_storage_bytes"

	service := []string{"resource.label.service_name"}
	errors := []string{"metric.label.response_code_class", "=", "5xx"}
	active := []string{"metric.label.state", "=", "active"}
	logErrors := []string{"metric.label.severity", "=", "ERROR"}

	target := func(ref, metric, aligner string, groups, filters []string, alias string) any {
		return projectTarget(ref, spec.ID, metric, "DELTA", "INT64", aligner, "REDUCE_SUM", groups, filters, alias)
	}

	serviceOverrides := []any{
		fieldOverride("telemetry-ingest-prod", "displayName", "telemetry-ingest-prod (Prod)"),
		fieldOverride("telemetry-ingest-prod", "color", map[string]any{"mode": "fixed", "fixedColor": "blue"}),
		fieldOverride("telemetry-ingest-staging", "displayName", "telemetry-ingest-staging (Staging)"),
		fieldOverride("telemetry-ingest-staging", "color", map[string]any{"mode": "fixed", "fixedColor": "purple"}),
	}
	errOverrides := []any{
		fieldOverride("telemetry-ingest-prod 5xx", "displayName", "telemetry-ingest-prod 5xx"),
		fieldOverride("telemetry-ingest-prod 5xx", "color", map[string]any{"mode": "fixed", "fixedColor": "orange"}),
		fieldOverride("telemetry-ingest-staging 5xx", "displayName", "telemetry-ingest-staging 5xx"),
		fieldOverride("telemetry-ingest-staging 5xx", "color", map[string]any{"mode": "fixed", "fixedColor": "red"}),
	}
	firestoreOverrides := []any{
		fieldOverride("Leituras", "displayName", "Leituras"),
		fieldOverride("Leituras", "color", map[string]any{"mode": "fixed", "fixedColor": "blue"}),
		fieldOverride("Escritas", "displayName", "Escritas"),
		fieldOverride("Escritas", "color", map[string]any{"mode": "fixed", "fixedColor": "orange"}),
	}

	panels := []any{
		textPanel(1, "", 0, 0, 24, 3, "## Telemetry Store & Ingest at a Glance\n\nProduction and staging · Ingestion API, scheduled jobs, and Firestore. **Totals reflect selected period; costs reflect indicated invoice.**"),
		childPanel(2, "Requisições · período", "stat", 0, 3, 6, 4, "short", "sum", "blue",
			"Soma das requisições Cloud Run recebidas no período para telemetry-ingest-prod e telemetry-ingest-staging, incluindo erros.",
			[]any{target("A", requests, "ALIGN_SUM", nil, nil, "Requisições")}, nil),
		childPanel(3, "Respostas 5xx", "stat", 6, 3, 6, 4, "short", "sum", "orange",
			"Quantidade de respostas com erro de servidor (HTTP 5xx). Ausência de série não comprova zero erros.",
			[]any{target("A", requests, "ALIGN_SUM", nil, errors, "Respostas 5xx")}, nil),
		childPanel(4, "Erros de rotina", "stat", 12, 3, 6, 4, "short", "sum", "orange",
			"Entradas de log com severidade ERROR emitidas pelo Cloud Scheduler no período selecionado.",
			[]any{projectTarget("A", spec.ID, schedulerLogs, "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", nil, logErrors, "Erros em logs")}, nil),
		childPanel(5, "Pior latência p95", "stat", 18, 3, 6, 4, "ms", "lastNotNull", "purple",
			"Pior latência p95 registrada entre os serviços da API no intervalo mais recente.",
			[]any{projectTarget("A", spec.ID, latencies, "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_MAX", nil, nil, "Pior p95")}, nil),

		overviewRow(20, "01  Operação · tráfego e falhas por ambiente", 7),
		childPanel(6, "Tráfego por serviço", "timeseries", 0, 8, 12, 8, "reqps", "", "",
			"Requisições por segundo diferenciando produção (prod) de homologação (staging).",
			[]any{target("A", requests, "ALIGN_RATE", service, nil, "{{resource.label.service_name}}")}, serviceOverrides),
		childPanel(7, "Erros de servidor por serviço", "timeseries", 12, 8, 12, 8, "reqps", "", "",
			"Respostas HTTP 5xx por segundo por serviço.",
			[]any{target("A", requests, "ALIGN_RATE", service, errors, "{{resource.label.service_name}} 5xx")}, errOverrides),
		childPanel(8, "Latência p95 por serviço", "timeseries", 0, 16, 12, 8, "ms", "", "",
			"Latência p95 em milissegundos por serviço ao longo do tempo.",
			[]any{projectTarget("A", spec.ID, latencies, "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_MAX", service, nil, "{{resource.label.service_name}} p95")}, serviceOverrides),
		childPanel(9, "Instâncias ativas por serviço", "timeseries", 12, 16, 12, 8, "short", "", "",
			"Média de instâncias ativas no intervalo. Escala a zero é esperada nos períodos sem tráfego.",
			[]any{projectTarget("A", spec.ID, instances, "GAUGE", "INT64", "ALIGN_MEAN", "REDUCE_SUM", service, active, "{{resource.label.service_name}}")}, serviceOverrides),

		overviewRow(21, "02  Rotinas e dados · o processamento está saudável?", 24),
		childPanel(10, "Logs de erro por rotina", "timeseries", 0, 25, 8, 9, "ops", "", "",
			"Taxa de logs com severidade ERROR emitidos pelas rotinas agendadas (Cloud Scheduler). Picos alertam falhas em jobs.",
			[]any{projectTarget("A", spec.ID, schedulerLogs, "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", []string{"resource.label.job_id"}, logErrors, "{{resource.label.job_id}}")}, nil),
		childPanel(11, "Firestore · ritmo de operações", "timeseries", 8, 25, 8, 9, "ops", "", "",
			"Taxa de leituras e escritas por segundo na base Firestore.",
			[]any{
				projectTarget("A", spec.ID, firestoreRead, "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", nil, nil, "Leituras"),
				projectTarget("B", spec.ID, firestoreWrite, "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", nil, nil, "Escritas"),
			}, firestoreOverrides),
		childPanel(12, "Firestore · armazenamento", "timeseries", 16, 25, 8, 9, "decbytes", "", "",
			"Bytes em disco ocupados por dados e índices no banco Firestore telemetry-store.",
			[]any{projectTarget("A", spec.ID, firestoreStorage, "GAUGE", "INT64", "ALIGN_MAX", "REDUCE_SUM", []string{"resource.label.database_id"}, nil, "{{resource.label.database_id}}")}, nil),

		overviewRow(22, "03  Custos · fatura disponível e atualização da fonte", 34),
	}

	summary := childBigQueryPanel(13, "Resumo da última fatura disponível", "table", 0, 35, 18, 5, "short", projectBillingSQL(spec.ID, "summary"), 1, nil,
		"Gross costs, applied credits, and net amount for the most recent invoice for Telemetry Store & Ingest.")
	freshness := childBigQueryPanel(14, "Defasagem do billing", "stat", 18, 35, 6, 5, "suffix: dias", projectBillingSQL(spec.ID, "freshness"), 1, freshnessThresholds(),
		"Dias desde a última atualização do export de custos da conta, medidos por export_time. Acima de 2 dias: atenção; a partir de 7: obsoleto.")
	servicesCost := childBigQueryPanel(15, "Custo por serviço · última fatura", "table", 0, 40, 12, 9, "short", projectBillingSQL(spec.ID, "services"), 1, nil,
		"Detalhamento de custo por serviço na última fatura disponível do projeto.")
	trend := childBigQueryPanel(16, "Custo líquido por mês de fatura", "barchart", 12, 40, 12, 9, "short", projectBillingSQL(spec.ID, "trend"), 1, nil,
		"Até 12 meses de fatura a partir da última fatura disponível. Valores na moeda indicada, sem conversão.")

	for _, p := range []map[string]any{summary, servicesCost} {
		p["options"] = map[string]any{"showHeader": true, "cellHeight": "md", "footer": map[string]any{"show": false}}
		fc := p["fieldConfig"].(map[string]any)
		fc["overrides"] = []any{
			fieldOverride("Servico", "custom.width", 210),
			fieldOverride("Fatura", "unit", "string"),
			fieldOverride("Moeda", "unit", "string"),
			fieldOverride("Moeda", "custom.width", 50),
			fieldOverride("Bruto", "custom.width", 60),
			fieldOverride("Creditos", "custom.width", 68),
			fieldOverride("Creditos", "displayName", "Créditos"),
			fieldOverride("Liquido", "custom.width", 68),
			fieldOverride("Liquido", "displayName", "Líquido"),
		}
	}
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["decimals"] = 0
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["displayName"] = "Defasagem"
	trend["options"] = map[string]any{"xField": "Fatura", "orientation": "vertical", "showValue": "never", "groupWidth": 0.7, "barWidth": 0.8, "xTickLabelRotation": -45, "legend": map[string]any{"showLegend": false}, "tooltip": map[string]any{"mode": "single", "sort": "none"}}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["custom"] = map[string]any{"fillOpacity": 85, "lineWidth": 0, "axisLabel": "Valor na moeda indicada"}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["color"] = map[string]any{"mode": "palette-classic"}

	panels = append(panels, summary, freshness, servicesCost, trend)
	note := textPanel(19, "", 0, 49, 24, 3, "**Como interpretar:** Sem dados ≠ zero. Staging e rotinas batelada escalam até zero quando inativas. Logs de erro de rotinas medem entradas com severidade ERROR. Custos são históricos do BigQuery export e podem apresentar defasagem de dias.")
	note["transparent"] = true
	panels = append(panels, note)

	serviceList := strings.Join(spec.Services, ", ")
	d := dashboard("Projeto — "+spec.Name, spec.UID, spec.Summary, panels, variables(spec.ID, spec.Environment, serviceList))
	d["time"] = map[string]any{"from": "now-24h", "to": "now"}
	d["refresh"] = "5m"
	d["links"] = projectNavLinks(spec.UID)
	return d
}

func projectDashboard(spec projectSpec) map[string]any {
	serviceList := strings.Join(spec.Services, ", ")
	header := fmt.Sprintf("## %s\n\n%s\n\n**Projeto:** `%s` · **Ambiente:** `%s` · **Serviços:** `%s` · [Voltar à visão geral](/d/unified-cloudrun-ops)", spec.Name, spec.Summary, spec.ID, spec.Environment, serviceList)
	panels := []any{
		textPanel(1, spec.Name, 0, 0, 24, 4, header),
		monitoringPanel(2, "Requisições por segundo", "stat", 0, 4, 6, 5, "reqps", []any{
			monitoringTarget("A", spec.ID, "run.googleapis.com/request_count", "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", nil, projectFilter(spec.ID, nil), "RPS"),
		}, nil),
		monitoringPanel(3, "Erros 5xx", "stat", 6, 4, 6, 5, "reqps", []any{
			monitoringTarget("A", spec.ID, "run.googleapis.com/request_count", "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", nil, projectFilter(spec.ID, []string{"metric.label.response_code_class", "=", "5xx"}), "5xx"),
		}, errorThresholds()),
		monitoringPanel(4, "Latência p95", "stat", 12, 4, 6, 5, "ms", []any{
			monitoringTarget("A", spec.ID, "run.googleapis.com/request_latencies", "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_PERCENTILE_95", nil, projectFilter(spec.ID, nil), "p95"),
		}, latencyThresholds()),
		monitoringPanel(5, "Instâncias ativas", "stat", 18, 4, 6, 5, "short", []any{
			monitoringTarget("A", spec.ID, "run.googleapis.com/container/instance_count", "GAUGE", "INT64", "ALIGN_MEAN", "REDUCE_SUM", nil, projectFilter(spec.ID, []string{"metric.label.state", "=", "active"}), "ativas"),
		}, nil),
		monitoringPanel(6, "Tráfego por serviço", "timeseries", 0, 9, 12, 8, "reqps", []any{
			monitoringTarget("A", spec.ID, "run.googleapis.com/request_count", "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", []string{"resource.label.service_name"}, projectFilter(spec.ID, nil), "{{resource.label.service_name}}"),
		}, nil),
		monitoringPanel(7, "Latência por serviço — p50/p95/p99", "timeseries", 12, 9, 12, 8, "ms", []any{
			monitoringTarget("A", spec.ID, "run.googleapis.com/request_latencies", "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_50", "REDUCE_PERCENTILE_50", []string{"resource.label.service_name"}, projectFilter(spec.ID, nil), "{{resource.label.service_name}} p50"),
			monitoringTarget("B", spec.ID, "run.googleapis.com/request_latencies", "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_PERCENTILE_95", []string{"resource.label.service_name"}, projectFilter(spec.ID, nil), "{{resource.label.service_name}} p95"),
			monitoringTarget("C", spec.ID, "run.googleapis.com/request_latencies", "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_99", "REDUCE_PERCENTILE_99", []string{"resource.label.service_name"}, projectFilter(spec.ID, nil), "{{resource.label.service_name}} p99"),
		}, latencyThresholds()),
		monitoringPanel(8, "CPU p95 por serviço", "timeseries", 0, 17, 12, 8, "percentunit", []any{
			monitoringTarget("A", spec.ID, "run.googleapis.com/container/cpu/utilizations", "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_PERCENTILE_95", []string{"resource.label.service_name"}, projectFilter(spec.ID, nil), "{{resource.label.service_name}}"),
		}, utilizationThresholds()),
		monitoringPanel(9, "Memória p95 por serviço", "timeseries", 12, 17, 12, 8, "percentunit", []any{
			monitoringTarget("A", spec.ID, "run.googleapis.com/container/memory/utilizations", "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_PERCENTILE_95", []string{"resource.label.service_name"}, projectFilter(spec.ID, nil), "{{resource.label.service_name}}"),
		}, utilizationThresholds()),
		bigQueryPanel(10, "Bruto, créditos e líquido · última fatura", "table", 0, 25, 12, 5, "short", billingSummarySQL(spec.ID), 1, nil),
		bigQueryPanel(11, "Custo líquido por mês de fatura", "timeseries", 12, 25, 12, 5, "short", billingTrendSQL(spec.ID), 0, nil),
	}
	panels = append(panels, extraPanels(spec)...)

	return dashboard("Projeto — "+spec.Name, spec.UID, spec.Summary, panels, variables(spec.ID, spec.Environment, serviceList))
}

func extraPanels(spec projectSpec) []any {
	switch spec.Extras {
	case "website":
		return []any{
			monitoringPanel(12, "Disponibilidade dos endpoints", "timeseries", 0, 30, 12, 8, "percentunit", []any{
				monitoringTarget("A", spec.ID, "monitoring.googleapis.com/uptime_check/check_passed", "GAUGE", "BOOL", "ALIGN_FRACTION_TRUE", "REDUCE_MEAN", []string{"metric.label.check_id"}, projectFilter(spec.ID, nil), "{{metric.label.check_id}}"),
			}, percentThresholds()),
			monitoringPanel(13, "Eventos de erro em logs", "timeseries", 12, 30, 12, 8, "ops", []any{
				monitoringTarget("A", spec.ID, "logging.googleapis.com/log_entry_count", "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", []string{"resource.type"}, projectFilter(spec.ID, []string{"metric.label.severity", "=", "ERROR"}), "{{resource.type}}"),
			}, errorThresholds()),
		}
	case "tasks-firestore":
		return []any{
			monitoringPanel(12, "Fila — tarefas aguardando", "timeseries", 0, 30, 8, 8, "short", []any{
				monitoringTarget("A", spec.ID, "cloudtasks.googleapis.com/queue/depth", "GAUGE", "INT64", "ALIGN_MAX", "REDUCE_MAX", []string{"resource.label.queue_id"}, projectFilter(spec.ID, nil), "{{resource.label.queue_id}}"),
			}, errorThresholds()),
			monitoringPanel(13, "Fila — tentativas por resposta", "timeseries", 8, 30, 8, 8, "ops", []any{
				monitoringTarget("A", spec.ID, "cloudtasks.googleapis.com/queue/task_attempt_count", "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", []string{"metric.label.response_code"}, projectFilter(spec.ID, nil), "{{metric.label.response_code}}"),
			}, nil),
			monitoringPanel(14, "Firestore — latência p95", "timeseries", 16, 30, 8, 8, "s", []any{
				monitoringTarget("A", spec.ID, "firestore.googleapis.com/api/request_latencies", "DELTA", "DISTRIBUTION", "ALIGN_PERCENTILE_95", "REDUCE_PERCENTILE_95", []string{"metric.label.api_method"}, projectFilter(spec.ID, nil), "{{metric.label.api_method}}"),
			}, latencyThresholds()),
		}
	case "bigquery":
		return []any{
			monitoringPanel(12, "BigQuery — bytes processados", "timeseries", 0, 30, 12, 8, "decbytes", []any{
				monitoringTarget("A", spec.ID, "bigquery.googleapis.com/query/scanned_bytes_billed", "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", []string{"metric.label.priority"}, projectFilter(spec.ID, nil), "{{metric.label.priority}}"),
			}, nil),
			monitoringPanel(13, "BigQuery — consultas executadas", "timeseries", 12, 30, 12, 8, "ops", []any{
				monitoringTarget("A", spec.ID, "bigquery.googleapis.com/query/execution_count", "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", []string{"metric.label.priority"}, projectFilter(spec.ID, nil), "{{metric.label.priority}}"),
			}, nil),
		}
	case "scheduler-firestore":
		return []any{
			monitoringPanel(12, "Schedulers — erros registrados", "timeseries", 0, 30, 8, 8, "ops", []any{
				monitoringTarget("A", spec.ID, "logging.googleapis.com/log_entry_count", "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", []string{"resource.label.job_id"}, projectFilter(spec.ID, []string{"metric.label.severity", "=", "ERROR"}), "{{resource.label.job_id}}"),
			}, errorThresholds()),
			monitoringPanel(13, "Firestore — operações", "timeseries", 8, 30, 8, 8, "ops", []any{
				monitoringTarget("A", spec.ID, "firestore.googleapis.com/document/read_ops_count", "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", nil, projectFilter(spec.ID, nil), "leituras"),
				monitoringTarget("B", spec.ID, "firestore.googleapis.com/document/write_ops_count", "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", nil, projectFilter(spec.ID, nil), "escritas"),
			}, nil),
			monitoringPanel(14, "Firestore — armazenamento", "timeseries", 16, 30, 8, 8, "decbytes", []any{
				monitoringTarget("A", spec.ID, "firestore.googleapis.com/storage/data_and_index_storage_bytes", "GAUGE", "INT64", "ALIGN_MAX", "REDUCE_SUM", []string{"resource.label.database_id"}, projectFilter(spec.ID, nil), "{{resource.label.database_id}}"),
			}, nil),
		}
	default:
		return nil
	}
}

func llmDashboard() map[string]any {
	const project = "enterprise-ai-gateway"
	const tokens = "aiplatform.googleapis.com/publisher/online_serving/token_count"
	const calls = "aiplatform.googleapis.com/publisher/online_serving/model_invocation_count"
	model := []string{"resource.label.model_user_id"}

	tokenOverrides := []any{
		fieldOverride("Entrada", "color", map[string]any{"mode": "fixed", "fixedColor": "purple"}),
		fieldOverride("Saída", "color", map[string]any{"mode": "fixed", "fixedColor": "cyan"}),
	}

	panels := []any{
		textPanel(1, "", 0, 0, 24, 3, "## Gateway de IA, em uma leitura\n\nVertex AI · Consumo, modelos e respostas. **Totais seguem o período selecionado; custos seguem a fatura indicada.**"),
		childPanel(2, "Tokens totais", "stat", 0, 3, 6, 4, "short", "sum", "purple",
			"Soma de todos os tokens (entrada e saída) processados via Vertex AI no período.",
			[]any{projectTarget("A", project, tokens, "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", nil, nil, "Tokens")}, nil),
		childPanel(3, "Tokens de entrada", "stat", 6, 3, 6, 4, "short", "sum", "blue",
			"Tokens de prompt enviados aos modelos Vertex AI no período.",
			[]any{projectTarget("A", project, tokens, "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", nil, []string{"metric.label.type", "=", "input"}, "Entrada")}, nil),
		childPanel(4, "Tokens de saída", "stat", 12, 3, 6, 4, "short", "sum", "cyan",
			"Tokens de resposta gerados pelos modelos Vertex AI no período.",
			[]any{projectTarget("A", project, tokens, "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", nil, []string{"metric.label.type", "=", "output"}, "Saída")}, nil),
		childPanel(5, "Chamadas de modelo", "stat", 18, 3, 6, 4, "short", "sum", "green",
			"Total de invocações Vertex AI no período selecionado, incluindo erros.",
			[]any{projectTarget("A", project, calls, "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", nil, nil, "Chamadas")}, nil),

		overviewRow(20, "01  Consumo · quanto e quais modelos?", 7),
		childPanel(6, "Tokens · entrada e saída", "bargauge", 0, 8, 8, 9, "short", "sum", "",
			"Totais de tokens no período. Entrada inclui o prompt/contexto; saída é o conteúdo gerado.",
			[]any{
				projectTarget("A", project, tokens, "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", nil, []string{"metric.label.type", "=", "input"}, "Entrada"),
				projectTarget("B", project, tokens, "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", nil, []string{"metric.label.type", "=", "output"}, "Saída"),
			}, tokenOverrides),
		childPanel(7, "Modelos chamados · participação", "piechart", 8, 8, 8, 9, "short", "sum", "",
			"Participação de cada modelo nas invocações no período. A legenda mostra quantidade e percentual.",
			[]any{projectTarget("A", project, calls, "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", model, nil, "{{resource.label.model_user_id}}")}, nil),
		childPanel(8, "Tokens consumidos por modelo", "bargauge", 16, 8, 8, 9, "short", "sum", "",
			"Entrada + saída somadas por modelo no período selecionado.",
			[]any{projectTarget("A", project, tokens, "DELTA", "INT64", "ALIGN_SUM", "REDUCE_SUM", model, nil, "{{resource.label.model_user_id}}")}, nil),

		childPanel(9, "Ritmo de consumo de tokens", "timeseries", 0, 17, 12, 8, "suffix: tokens/s", "", "",
			"Taxa de consumo de tokens por segundo ao longo do tempo (entrada e saída).",
			[]any{
				projectTarget("A", project, tokens, "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", nil, []string{"metric.label.type", "=", "input"}, "Entrada"),
				projectTarget("B", project, tokens, "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", nil, []string{"metric.label.type", "=", "output"}, "Saída"),
			}, tokenOverrides),
		childPanel(10, "Chamadas por modelo ao longo do tempo", "timeseries", 12, 17, 12, 8, "reqps", "", "",
			"Invocações por segundo por modelo Vertex AI ao longo do tempo.",
			[]any{projectTarget("A", project, calls, "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", model, nil, "{{resource.label.model_user_id}}")}, nil),

		overviewRow(21, "02  Respostas · há falhas ou lentidão?", 25),
		childPanel(11, "Ritmo de invocações por resposta", "timeseries", 0, 26, 12, 8, "ops", "", "",
			"Taxa de invocações por código de resposta HTTP. Picos de 429 indicam saturação de cota.",
			[]any{projectTarget("A", project, calls, "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", []string{"metric.label.response_code"}, nil, "{{metric.label.response_code}}")}, nil),
		childPanel(12, "Erros por categoria de falha", "timeseries", 12, 26, 12, 8, "ops", "", "",
			"Taxa de erros classificados por categoria oficial da API (ex: RESOURCE_EXHAUSTED, INVALID_ARGUMENT).",
			[]any{projectTarget("A", project, calls, "DELTA", "INT64", "ALIGN_RATE", "REDUCE_SUM", []string{"metric.label.error_category"}, nil, "{{metric.label.error_category}}")}, nil),

		overviewRow(22, "03  Custos · fatura disponível e atualização da fonte", 34),
	}

	summary := childBigQueryPanel(13, "Resumo da última fatura disponível", "table", 0, 35, 18, 5, "short", projectBillingSQL(project, "summary"), 1, nil,
		"Custos brutos, créditos aplicados e valor líquido da fatura mais recente para o Gateway de IA.")
	freshness := childBigQueryPanel(14, "Defasagem do billing", "stat", 18, 35, 6, 5, "suffix: dias", projectBillingSQL(project, "freshness"), 1, freshnessThresholds(),
		"Dias desde a última atualização do export de custos da conta, medidos por export_time. Acima de 2 dias: atenção; a partir de 7: obsoleto.")
	servicesCost := childBigQueryPanel(15, "Custo por serviço · última fatura", "table", 0, 40, 12, 9, "short", projectBillingSQL(project, "services"), 1, nil,
		"Detalhamento de custo por serviço na última fatura disponível do projeto.")
	trend := childBigQueryPanel(16, "Custo líquido por mês de fatura", "barchart", 12, 40, 12, 9, "short", projectBillingSQL(project, "trend"), 1, nil,
		"Até 12 meses de fatura a partir da última fatura disponível. Valores na moeda indicada, sem conversão.")

	for _, p := range []map[string]any{summary, servicesCost} {
		p["options"] = map[string]any{"showHeader": true, "cellHeight": "md", "footer": map[string]any{"show": false}}
		fc := p["fieldConfig"].(map[string]any)
		fc["overrides"] = []any{
			fieldOverride("Servico", "custom.width", 210),
			fieldOverride("Fatura", "unit", "string"),
			fieldOverride("Moeda", "unit", "string"),
			fieldOverride("Moeda", "custom.width", 50),
			fieldOverride("Bruto", "custom.width", 60),
			fieldOverride("Creditos", "custom.width", 68),
			fieldOverride("Creditos", "displayName", "Créditos"),
			fieldOverride("Liquido", "custom.width", 68),
			fieldOverride("Liquido", "displayName", "Líquido"),
		}
	}
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["decimals"] = 0
	freshness["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["displayName"] = "Defasagem"
	trend["options"] = map[string]any{"xField": "Fatura", "orientation": "vertical", "showValue": "never", "groupWidth": 0.7, "barWidth": 0.8, "xTickLabelRotation": -45, "legend": map[string]any{"showLegend": false}, "tooltip": map[string]any{"mode": "single", "sort": "none"}}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["custom"] = map[string]any{"fillOpacity": 85, "lineWidth": 0, "axisLabel": "Valor na moeda indicada"}
	trend["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["color"] = map[string]any{"mode": "palette-classic"}

	panels = append(panels, summary, freshness, servicesCost, trend)
	note := textPanel(19, "", 0, 49, 24, 3, "**Como interpretar:** Sem dados ≠ zero. O gateway não possui instâncias computacionais contínuas alocadas. Chamadas e tokens cobrem exclusivamente o consumo Vertex AI/Gemini registrado. Custos são históricos do BigQuery export e podem apresentar defasagem de dias.")
	note["transparent"] = true
	panels = append(panels, note)

	d := dashboard("Projeto — Gateway de IA", "aiops-ai-gateway", "Tokens, modelos, chamadas e custos do projeto de cotas Vertex AI.", panels, variables(project, "shared", "vertex-ai"))
	d["time"] = map[string]any{"from": "now-24h", "to": "now"}
	d["refresh"] = "5m"
	d["links"] = projectNavLinks("aiops-ai-gateway")
	return d
}

func dashboard(title, uid, description string, panels []any, vars []any) map[string]any {
	return map[string]any{
		"annotations":          map[string]any{"list": []any{}},
		"description":          description,
		"editable":             true,
		"fiscalYearStartMonth": 0,
		"graphTooltip":         1,
		"id":                   nil,
		"links":                []any{},
		"liveNow":              false,
		"panels":               panels,
		"refresh":              "1m",
		"schemaVersion":        39,
		"tags":                 []string{"ai-ops", "gcp", "human-first", "read-only"},
		"templating":           map[string]any{"list": vars},
		"time":                 map[string]any{"from": "now-6h", "to": "now"},
		"timepicker":           map[string]any{"refresh_intervals": []string{"30s", "1m", "5m", "15m", "30m", "1h"}},
		"timezone":             "browser",
		"title":                title,
		"uid":                  uid,
		"version":              1,
		"weekStart":            "",
	}
}

func variables(project, environment, services string) []any {
	return []any{
		constantVariable("project_id", project),
		constantVariable("environment", environment),
		constantVariable("service_scope", services),
	}
}

func constantVariable(name, value string) map[string]any {
	return map[string]any{
		"name": name, "label": name, "type": "constant", "hide": 2,
		"query": value, "current": map[string]any{"text": value, "value": value},
	}
}

func textPanel(id int, title string, x, y, w, h int, markdown string) map[string]any {
	return map[string]any{
		"id": id, "title": title, "type": "text", "gridPos": grid(x, y, w, h),
		"options": map[string]any{"mode": "markdown", "content": markdown},
	}
}

func monitoringPanel(id int, title, panelType string, x, y, w, h int, unit string, targets []any, thresholds map[string]any) map[string]any {
	noValue := "0"
	if unit == "reqps" || strings.Contains(title, "Erros 5xx") {
		noValue = "0 req/s"
	} else if strings.Contains(strings.ToLower(title), "instâncias") || strings.Contains(strings.ToLower(title), "capacidade") {
		noValue = "0 ativas"
	}
	defaults := map[string]any{
		"unit":        unit,
		"displayName": "${__series.name}",
		"noValue":     noValue,
		"color":       map[string]any{"mode": "thresholds"},
		"custom": map[string]any{
			"drawStyle": "line", "lineInterpolation": "smooth", "lineWidth": 2,
			"fillOpacity": 12, "showPoints": "never", "spanNulls": true,
		},
	}
	if thresholds != nil {
		defaults["thresholds"] = thresholds
	}
	panel := map[string]any{
		"id": id, "title": title, "type": panelType, "gridPos": grid(x, y, w, h),
		"datasource":  monitoringDS,
		"targets":     targets,
		"fieldConfig": map[string]any{"defaults": defaults, "overrides": []any{}},
	}
	if panelType == "stat" {
		panel["options"] = map[string]any{
			"reduceOptions": map[string]any{"values": false, "calcs": []string{"lastNotNull"}, "fields": ""},
			"orientation":   "auto", "textMode": "auto", "colorMode": "value", "graphMode": "area", "justifyMode": "auto",
		}
	} else {
		panel["options"] = map[string]any{
			"legend":  map[string]any{"displayMode": "list", "placement": "bottom", "calcs": []any{}},
			"tooltip": map[string]any{"mode": "multi", "sort": "desc"},
		}
	}
	return panel
}

func bigQueryPanel(id int, title, panelType string, x, y, w, h int, unit, sql string, format int, thresholds map[string]any) map[string]any {
	noValue := "0"
	if unit == "reqps" {
		noValue = "0 req/s"
	}
	defaults := map[string]any{
		"unit":    unit,
		"noValue": noValue,
		"color":   map[string]any{"mode": "thresholds"},
	}
	if thresholds != nil {
		defaults["thresholds"] = thresholds
	}
	panel := map[string]any{
		"id": id, "title": title, "type": panelType, "gridPos": grid(x, y, w, h),
		"datasource": billingDS,
		"targets": []any{map[string]any{
			"datasource": billingDS, "editorMode": "code", "format": format, "location": "US",
			"rawQuery": true, "rawSql": sql, "refId": "A",
		}},
		"fieldConfig": map[string]any{"defaults": defaults, "overrides": []any{}},
	}
	if strings.Contains(strings.ToLower(title), "tendência") {
		panel["timeFrom"] = "now-1y"
	}
	if panelType == "stat" {
		panel["options"] = map[string]any{
			"reduceOptions": map[string]any{"values": false, "calcs": []string{"lastNotNull"}, "fields": ""},
			"orientation":   "horizontal", "textMode": "auto", "colorMode": "value", "graphMode": "none", "justifyMode": "auto",
		}
	} else if panelType == "table" {
		panel["options"] = map[string]any{"showHeader": true, "cellHeight": "sm", "footer": map[string]any{"show": false, "reducer": []string{"sum"}}}
	} else {
		panel["options"] = map[string]any{
			"legend":  map[string]any{"displayMode": "table", "placement": "bottom", "calcs": []string{"lastNotNull"}},
			"tooltip": map[string]any{"mode": "multi", "sort": "desc"},
		}
	}
	return panel
}

func projectFilter(projectID string, existing []string) []string {
	res := []string{"resource.label.project_id", "=", projectID}
	if len(existing) == 0 {
		return res
	}
	res = append(res, "AND")
	res = append(res, existing...)
	return res
}

func monitoringTarget(ref, project, metric, kind, valueType, aligner, reducer string, groupBy, filters []string, alias string) map[string]any {
	query := map[string]any{
		"projectName": project, "metricType": metric, "metricKind": kind, "valueType": valueType,
		"perSeriesAligner": aligner, "alignmentPeriod": "stackdriver-auto",
	}
	if reducer != "" {
		query["crossSeriesReducer"] = reducer
	}
	if len(groupBy) > 0 {
		query["groupBys"] = groupBy
	}
	if len(filters) > 0 {
		query["filters"] = filters
	}
	return map[string]any{
		"datasource": monitoringDS, "queryType": "metrics", "refId": ref,
		"aliasBy": alias, "metricQuery": query,
	}
}

func grid(x, y, w, h int) map[string]int {
	return map[string]int{"x": x, "y": y, "w": w, "h": h}
}

func billingBaseSQL(projectFilter string) string {
	filter := ""
	if projectFilter != "" {
		filter = fmt.Sprintf("WHERE project.id = '%s'", strings.ReplaceAll(projectFilter, "'", ""))
	}
	return fmt.Sprintf(`WITH billing AS (
  SELECT
    invoice.month AS invoice_month,
    project.id AS project_id,
    currency,
    cost,
    COALESCE((SELECT SUM(credit.amount) FROM UNNEST(credits) AS credit), 0) AS credit_amount
  FROM %s
  %s
)`, billingTable, filter)
}

func billingSummarySQL(projectFilter string) string {
	return billingBaseSQL(projectFilter) + `,
latest AS (SELECT MAX(invoice_month) AS invoice_month FROM billing)
SELECT
  FORMAT_DATE('%m/%Y', PARSE_DATE('%Y%m', invoice_month)) AS Fatura,
  currency AS Moeda,
  ROUND(SUM(cost), 2) AS Bruto,
  ROUND(-SUM(credit_amount), 2) AS Creditos,
  ROUND(SUM(cost + credit_amount), 2) AS Liquido
FROM billing
WHERE invoice_month = (SELECT invoice_month FROM latest)
GROUP BY invoice_month, currency
ORDER BY currency`
}

func billingFreshnessSQL() string {
	return fmt.Sprintf(`SELECT
  DATE_DIFF(CURRENT_DATE('America/Sao_Paulo'), DATE(MAX(export_time), 'America/Sao_Paulo'), DAY) AS dias_sem_atualizacao
FROM %s
`, billingTable)
}

func billingByProjectSQL() string {
	return billingBaseSQL("") + `,
latest AS (SELECT MAX(invoice_month) AS invoice_month FROM billing)
SELECT
  COALESCE(project_id, 'sem-projeto') AS projeto,
  currency AS moeda,
  ROUND(SUM(cost), 2) AS custo_bruto,
  ROUND(-SUM(credit_amount), 2) AS creditos_aplicados,
  ROUND(SUM(cost + credit_amount), 2) AS custo_liquido
FROM billing
WHERE invoice_month = (SELECT invoice_month FROM latest)
GROUP BY projeto, currency
ORDER BY custo_liquido DESC`
}

func billingTrendSQL(projectFilter string) string {
	return billingBaseSQL(projectFilter) + `
SELECT
  TIMESTAMP(PARSE_DATE('%Y%m', invoice_month), 'America/Sao_Paulo') AS time,
  CONCAT('Líquido · ', currency) AS metric,
  ROUND(SUM(cost + credit_amount), 2) AS value
FROM billing
GROUP BY time, metric
ORDER BY time, metric`
}

func thresholds(steps ...map[string]any) map[string]any {
	items := make([]any, 0, len(steps))
	for _, step := range steps {
		items = append(items, step)
	}
	return map[string]any{"mode": "absolute", "steps": items}
}

func step(color string, value any) map[string]any {
	return map[string]any{"color": color, "value": value}
}

func percentThresholds() map[string]any {
	return thresholds(step("red", nil), step("yellow", 0.95), step("green", 0.99))
}

func errorThresholds() map[string]any {
	return thresholds(step("green", nil), step("yellow", 0.01), step("red", 0.1))
}

func latencyThresholds() map[string]any {
	return thresholds(step("green", nil), step("yellow", 500), step("red", 1500))
}

func utilizationThresholds() map[string]any {
	return thresholds(step("green", nil), step("yellow", 0.7), step("red", 0.9))
}

func freshnessThresholds() map[string]any {
	return thresholds(step("gray", nil), step("yellow", 2), step("red", 7))
}
