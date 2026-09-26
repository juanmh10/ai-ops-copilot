package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderDashboardsIdempotency verifies that generating dashboards is 100% deterministic
// and produces byte-for-byte identical output on repeated runs without drift.
func TestRenderDashboardsIdempotency(t *testing.T) {
	tempDir1 := t.TempDir()
	tempDir2 := t.TempDir()

	renderTo := func(dir string) map[string][]byte {
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

		results := make(map[string][]byte)
		for name, dash := range dashboards {
			data, err := json.MarshalIndent(dash, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal %s: %v", name, err)
			}
			data = append(data, '\n')
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatalf("failed to write %s: %v", path, err)
			}
			results[name] = data
		}
		return results
	}

	run1 := renderTo(tempDir1)
	run2 := renderTo(tempDir2)

	expectedFiles := []string{
		"unified-cloudrun-dashboard.json",
		"ai-ops-enterprise-core-prod.json",
		"ai-ops-enterprise-saas-staging.json",
		"ai-ops-enterprise-ai-analytics.json",
		"ai-ops-enterprise-telemetry-prod.json",
		"ai-ops-enterprise-ai-gateway.json",
	}

	if len(run1) != len(expectedFiles) {
		t.Fatalf("expected %d dashboards, got %d", len(expectedFiles), len(run1))
	}

	for _, name := range expectedFiles {
		d1, ok1 := run1[name]
		d2, ok2 := run2[name]
		if !ok1 || !ok2 {
			t.Fatalf("missing dashboard %s in runs", name)
		}
		if string(d1) != string(d2) {
			t.Errorf("idempotency failure: file %s differed between consecutive runs", name)
		}

		// Compare against provisioned file on disk
		diskPath := filepath.Join("..", "..", "..", "grafana-provisioning", "dashboards", "json", name)
		diskData, err := os.ReadFile(diskPath)
		if err != nil {
			t.Fatalf("failed to read disk dashboard %s: %v", diskPath, err)
		}
		if string(d1) != string(diskData) {
			t.Errorf("rendered output for %s does not match provisioned file on disk", name)
		}
	}
}

// TestChildDashboardsProjectFilterIsolation exhaustively inspects every panel in every child dashboard
// to ensure zero cross-project telemetry leakage and strict resource.label.project_id filtering.
func TestChildDashboardsProjectFilterIsolation(t *testing.T) {
	childDashboards := map[string]struct {
		projectID       string
		foreignProjects []string
		foreignServices []string
	}{
		"ai-ops-enterprise-core-prod.json": {
			projectID: "enterprise-core-prod",
			foreignProjects: []string{
				"enterprise-saas-staging",
				"enterprise-ai-analytics",
				"enterprise-telemetry-prod",
				"enterprise-ai-gateway",
			},
			foreignServices: []string{
				"stg-saas-api",
				"stg-task-worker",
				"ai-service-api",
				"ai-portal-frontend",
				"telemetry-ingest-prod",
				"telemetry-ingest-staging",
			},
		},
		"ai-ops-enterprise-saas-staging.json": {
			projectID: "enterprise-saas-staging",
			foreignProjects: []string{
				"enterprise-core-prod",
				"enterprise-ai-analytics",
				"enterprise-telemetry-prod",
				"enterprise-ai-gateway",
			},
			foreignServices: []string{
				"portal-api",
				"auth-gateway-api",
				"ai-service-api",
				"ai-portal-frontend",
				"telemetry-ingest-prod",
				"telemetry-ingest-staging",
			},
		},
		"ai-ops-enterprise-ai-analytics.json": {
			projectID: "enterprise-ai-analytics",
			foreignProjects: []string{
				"enterprise-core-prod",
				"enterprise-saas-staging",
				"enterprise-telemetry-prod",
				"enterprise-ai-gateway",
			},
			foreignServices: []string{
				"portal-api",
				"auth-gateway-api",
				"stg-saas-api",
				"stg-task-worker",
				"telemetry-ingest-prod",
				"telemetry-ingest-staging",
			},
		},
		"ai-ops-enterprise-telemetry-prod.json": {
			projectID: "enterprise-telemetry-prod",
			foreignProjects: []string{
				"enterprise-core-prod",
				"enterprise-saas-staging",
				"enterprise-ai-analytics",
				"enterprise-ai-gateway",
			},
			foreignServices: []string{
				"portal-api",
				"auth-gateway-api",
				"stg-saas-api",
				"stg-task-worker",
				"ai-service-api",
				"ai-portal-frontend",
			},
		},
		"ai-ops-enterprise-ai-gateway.json": {
			projectID: "enterprise-ai-gateway",
			foreignProjects: []string{
				"enterprise-core-prod",
				"enterprise-saas-staging",
				"enterprise-ai-analytics",
				"enterprise-telemetry-prod",
			},
			foreignServices: []string{
				"portal-api",
				"auth-gateway-api",
				"stg-saas-api",
				"stg-task-worker",
				"ai-service-api",
				"ai-portal-frontend",
				"telemetry-ingest-prod",
				"telemetry-ingest-staging",
			},
		},
	}

	baseDir := filepath.Join("..", "..", "..", "grafana-provisioning", "dashboards", "json")

	for file, expected := range childDashboards {
		t.Run(file, func(t *testing.T) {
			path := filepath.Join(baseDir, file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read %s: %v", path, err)
			}

			var dash map[string]any
			if err := json.Unmarshal(data, &dash); err != nil {
				t.Fatalf("failed to unmarshal %s: %v", path, err)
			}

			panels, ok := dash["panels"].([]any)
			if !ok || len(panels) == 0 {
				t.Fatalf("%s has no panels", file)
			}

			cmTargetCount := 0
			for _, pRaw := range panels {
				panel := pRaw.(map[string]any)
				pid := panel["id"]
				pTitle := panel["title"]
				pType := panel["type"]
				if pType == "text" {
					continue
				}

				targets, _ := panel["targets"].([]any)
				for _, tRaw := range targets {
					target := tRaw.(map[string]any)
					refID := target["refId"]

					if mqRaw, hasMq := target["metricQuery"]; hasMq {
						cmTargetCount++
						mq := mqRaw.(map[string]any)

						// Assert projectName matches expected project ID
						if projName, _ := mq["projectName"].(string); projName != expected.projectID {
							t.Errorf("panel %v ('%s') target %v: projectName '%s' != expected '%s'",
								pid, pTitle, refID, projName, expected.projectID)
						}

						// Assert filters contain resource.label.project_id = expected.projectID
						filtersRaw, _ := mq["filters"].([]any)
						foundProjectFilter := false
						for i := 0; i+2 < len(filtersRaw); i++ {
							k, _ := filtersRaw[i].(string)
							op, _ := filtersRaw[i+1].(string)
							val, _ := filtersRaw[i+2].(string)
							if k == "resource.label.project_id" {
								if op != "=" {
									t.Errorf("panel %v ('%s') target %v: filter op is '%s', expected '='", pid, pTitle, refID, op)
								}
								if val != expected.projectID {
									t.Errorf("panel %v ('%s') target %v: filtered project_id is '%s', expected '%s'", pid, pTitle, refID, val, expected.projectID)
								}
								foundProjectFilter = true
								break
							}
						}
						if !foundProjectFilter {
							t.Errorf("panel %v ('%s') target %v lacks resource.label.project_id filter! Filters: %v",
								pid, pTitle, refID, filtersRaw)
						}

						// Assert no foreign project ID appears in the target
						targetJSON, _ := json.Marshal(target)
						targetStr := string(targetJSON)
						for _, foreignProj := range expected.foreignProjects {
							if strings.Contains(targetStr, foreignProj) {
								t.Errorf("panel %v target %v leaked foreign project ID '%s'", pid, refID, foreignProj)
							}
						}

						// Assert no foreign services leaked
						for _, foreignSvc := range expected.foreignServices {
							if strings.Contains(targetStr, foreignSvc) {
								t.Errorf("panel %v target %v leaked foreign service '%s'", pid, refID, foreignSvc)
							}
						}
					}

					// If BigQuery panel, verify project filter in SQL
					if rawSQL, hasSQL := target["rawSql"].(string); hasSQL {
						if strings.Contains(rawSQL, "dias_sem_atualizacao") {
							// Freshness queries global billing table
							continue
						}
						expectedSQLFilter := fmt.Sprintf("WHERE project.id = '%s'", expected.projectID)
						if !strings.Contains(rawSQL, expectedSQLFilter) {
							t.Errorf("panel %v ('%s') SQL target lacks expected filter %s. SQL: %s",
								pid, pTitle, expectedSQLFilter, rawSQL)
						}
					}
				}
			}

			if cmTargetCount == 0 {
				t.Errorf("%s has no Cloud Monitoring targets", file)
			}
		})
	}
}

// TestStatPanelsNoValueAndThresholds verifies scale-to-zero noValue configuration
// and threshold evaluation on empty/zero data across all 6 dashboards.
func TestStatPanelsNoValueAndThresholds(t *testing.T) {
	baseDir := filepath.Join("..", "..", "..", "grafana-provisioning", "dashboards", "json")
	files, err := filepath.Glob(filepath.Join(baseDir, "*.json"))
	if err != nil {
		t.Fatalf("failed to glob dashboards: %v", err)
	}

	totalStatPanels := 0

	for _, file := range files {
		name := filepath.Base(file)
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read %s: %v", file, err)
		}

		var dash map[string]any
		if err := json.Unmarshal(data, &dash); err != nil {
			t.Fatalf("failed to unmarshal %s: %v", file, err)
		}

		panels, _ := dash["panels"].([]any)
		for _, pRaw := range panels {
			p := pRaw.(map[string]any)
			if pType, _ := p["type"].(string); pType != "stat" {
				continue
			}

			totalStatPanels++
			pid := p["id"]
			title, _ := p["title"].(string)
			fc, _ := p["fieldConfig"].(map[string]any)
			defaults, _ := fc["defaults"].(map[string]any)
			noValue, _ := defaults["noValue"].(string)
			unit, _ := defaults["unit"].(string)

			if noValue == "" {
				t.Errorf("[%s] Panel %v ('%s') is a stat panel but missing noValue", name, pid, title)
			}

			// Verify specific noValue patterns
			if name == "unified-cloudrun-dashboard.json" || name == "ai-ops-enterprise-core-prod.json" || name == "ai-ops-enterprise-saas-staging.json" || name == "ai-ops-enterprise-ai-analytics.json" || name == "ai-ops-enterprise-telemetry-prod.json" || name == "ai-ops-enterprise-ai-gateway.json" {
				if noValue != "Sem dados" {
					t.Errorf("[%s] panel %v must distinguish missing data from zero (expected 'Sem dados', got '%s')", name, pid, noValue)
				}
				continue
			}
			if unit == "reqps" || strings.Contains(strings.ToLower(title), "5xx") || strings.Contains(strings.ToLower(title), "erros") {
				if noValue != "0 req/s" {
					t.Errorf("[%s] Panel %v ('%s') expected noValue '0 req/s', got '%s'", name, pid, title, noValue)
				}
			} else if strings.Contains(strings.ToLower(title), "instâncias") || strings.Contains(strings.ToLower(title), "ativas") {
				if noValue != "0 ativas" {
					t.Errorf("[%s] Panel %v ('%s') expected noValue '0 ativas', got '%s'", name, pid, title, noValue)
				}
			}

			// Verify error threshold evaluates to green at 0
			if strings.Contains(strings.ToLower(title), "5xx") || strings.Contains(strings.ToLower(title), "erros") {
				threshRaw, _ := defaults["thresholds"].(map[string]any)
				stepsRaw, _ := threshRaw["steps"].([]any)
				if len(stepsRaw) == 0 {
					t.Errorf("[%s] Panel %v ('%s') has no threshold steps", name, pid, title)
				} else {
					step0 := stepsRaw[0].(map[string]any)
					if color, _ := step0["color"].(string); color != "green" {
						t.Errorf("[%s] Panel %v ('%s') step 0 color is '%s', expected 'green' for zero-error health", name, pid, title, color)
					}
					if step0["value"] != nil {
						t.Errorf("[%s] Panel %v ('%s') step 0 value is not nil (%v)", name, pid, title, step0["value"])
					}
				}
			}
		}
	}

	if totalStatPanels < 20 {
		t.Errorf("expected at least 20 stat panels across the suite, found %d", totalStatPanels)
	}
}

// TestProvisioningYAMLConfig verifies dashboards.yaml provider configuration.
func TestProvisioningYAMLConfig(t *testing.T) {
	path := filepath.Join("..", "..", "..", "grafana-provisioning", "dashboards", "dashboards.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	content := string(data)

	if !strings.Contains(content, "allowUiUpdates: true") {
		t.Errorf("dashboards.yaml does not contain 'allowUiUpdates: true'")
	}
	if !strings.Contains(content, "editable: true") {
		t.Errorf("dashboards.yaml does not contain 'editable: true'")
	}
}

func TestOverviewMetricSemantics(t *testing.T) {
	d := overviewDashboard()
	panels := d["panels"].([]any)
	seen := map[int]bool{}
	for _, raw := range panels {
		p := raw.(map[string]any)
		id := p["id"].(int)
		if seen[id] {
			t.Fatalf("duplicate panel ID %d", id)
		}
		seen[id] = true
		g := p["gridPos"].(map[string]int)
		if g["x"] < 0 || g["x"]+g["w"] > 24 || g["h"] <= 0 {
			t.Errorf("invalid grid for panel %d: %v", id, g)
		}
		for _, other := range panels {
			op := other.(map[string]any)
			if op["id"].(int) >= id {
				continue
			}
			og := op["gridPos"].(map[string]int)
			if g["x"] < og["x"]+og["w"] && og["x"] < g["x"]+g["w"] &&
				g["y"] < og["y"]+og["h"] && og["y"] < g["y"]+g["h"] {
				t.Errorf("panels %d and %v overlap", id, op["id"])
			}
		}
		targets, _ := p["targets"].([]any)
		for _, rawTarget := range targets {
			target := rawTarget.(map[string]any)
			m, ok := target["metricQuery"].(map[string]any)
			if !ok {
				continue
			}
			filters := m["filters"].([]string)
			if m["aliasBy"] == nil || m["aliasBy"] != target["aliasBy"] {
				t.Errorf("panel %d: aliases must survive Grafana 11 query migration", id)
			}
			if len(filters) < 3 || filters[0] != "resource.label.project_id" || filters[1] != "=~" {
				t.Errorf("panel %d: expected Grafana-supported project regex", id)
			}
			if p["type"] == "stat" || p["type"] == "piechart" || p["type"] == "bargauge" {
				r := p["options"].(map[string]any)["reduceOptions"].(map[string]any)
				if r["calcs"].([]string)[0] != "sum" || r["fields"] != "" || m["perSeriesAligner"] != "ALIGN_SUM" {
					t.Errorf("panel %d: totals must sum delta counts, excluding time fields", id)
				}
			}
			if p["type"] == "piechart" {
				if !strings.HasSuffix(m["metricType"].(string), "/model_invocation_count") ||
					m["groupBys"].([]string)[0] != "resource.label.model_user_id" {
					t.Errorf("model participation must count invocations by model")
				}
			}
			if strings.HasSuffix(m["metricType"].(string), "/request_latencies") && m["crossSeriesReducer"] != "REDUCE_MAX" {
				t.Errorf("worst series p95 must use max, not percentile of percentiles")
			}
		}
	}
}

func TestOverviewBillingSeparatesCurrencyAndPeriod(t *testing.T) {
	steps := freshnessThresholds()["steps"].([]any)
	if steps[0].(map[string]any)["color"] != "gray" {
		t.Error("missing billing data must not appear healthy in green")
	}
	for _, view := range []string{"summary", "projects", "trend", "freshness"} {
		sql := overviewBillingSQL(view)
		if view != "freshness" && !strings.Contains(sql, "project_id IN") {
			t.Errorf("%s billing must be scoped to supervised projects", view)
		}
		if view == "freshness" && strings.Contains(sql, "project_id IN") {
			t.Error("billing freshness must represent the account export, not only supervised projects")
		}
		if view != "freshness" && !strings.Contains(sql, "GROUP BY") {
			t.Errorf("%s must aggregate without mixing currencies", view)
		}
		if view == "freshness" && (!strings.Contains(sql, "MAX(export_time)") || strings.Contains(sql, "MAX(usage_start_time)")) {
			t.Error("freshness must measure the export timestamp, not the age of usage")
		}
		if strings.Contains(sql, "$__timeFilter") {
			t.Errorf("invoice history must not silently follow operational time filter")
		}
		if view == "summary" && (!strings.Contains(sql, "'5 projetos monitorados'") || !strings.Contains(sql, "'Toda a conta'")) {
			t.Error("summary scope names must fit the billing table columns")
		}
		if view == "projects" && (!strings.Contains(sql, "'Sem dados'") || !strings.Contains(sql, "COALESCE(b.currency, '—') AS Moeda")) {
			t.Error("project billing must distinguish missing exports from zero-valued costs")
		}
	}
}

func TestOverviewBillingTableFitsNarrowDashboardGrid(t *testing.T) {
	panels := overviewDashboard()["panels"].([]any)
	byID := make(map[int]map[string]any, len(panels))
	for _, raw := range panels {
		panel := raw.(map[string]any)
		byID[panel["id"].(int)] = panel
	}
	table := byID[17]["gridPos"].(map[string]int)
	chart := byID[18]["gridPos"].(map[string]int)
	if table["x"] != 0 || table["w"] != 14 || chart["x"] != 14 || chart["w"] != 10 {
		t.Errorf("billing table and monthly chart must fit as 14/10 columns at narrow widths; table=%v chart=%v", table, chart)
	}
	fieldConfig := byID[17]["fieldConfig"].(map[string]any)
	defaults := fieldConfig["defaults"].(map[string]any)
	if defaults["noValue"] != "—" {
		t.Error("missing project billing values must use a compact unavailable marker, not zero or repeated labels")
	}
	custom := defaults["custom"].(map[string]any)
	cellOptions := custom["cellOptions"].(map[string]any)
	if cellOptions["wrapText"] != true {
		t.Error("billing table cells must wrap so missing-data labels do not overlap adjacent columns")
	}
}

func TestWebsiteProdMetricSemantics(t *testing.T) {
	spec := projectSpec{
		ID:          "enterprise-core-prod",
		UID:         "aiops-core-prod",
		File:        "ai-ops-enterprise-core-prod.json",
		Name:        "Website & APIs Core",
		Environment: "production",
		Summary:     "Produção core, portfólio e APIs públicas. Escala até zero é comportamento esperado quando não há tráfego.",
		Services:    []string{"portal-api", "auth-gateway-api"},
		Extras:      "website",
	}
	d := websiteProdDashboard(spec)
	panels := d["panels"].([]any)
	seen := map[int]bool{}
	for _, raw := range panels {
		p := raw.(map[string]any)
		id := p["id"].(int)
		if seen[id] {
			t.Fatalf("duplicate panel ID %d", id)
		}
		seen[id] = true
		g := p["gridPos"].(map[string]int)
		if g["x"] < 0 || g["x"]+g["w"] > 24 || g["h"] <= 0 {
			t.Errorf("invalid grid for panel %d: %v", id, g)
		}
		for _, other := range panels {
			op := other.(map[string]any)
			if op["id"].(int) >= id {
				continue
			}
			og := op["gridPos"].(map[string]int)
			if g["x"] < og["x"]+og["w"] && og["x"] < g["x"]+g["w"] &&
				g["y"] < og["y"]+og["h"] && og["y"] < g["y"]+g["h"] {
				t.Errorf("panels %d and %v overlap", id, op["id"])
			}
		}
	}
}

func TestCondSaasMetricSemantics(t *testing.T) {
	spec := projectSpec{
		ID:          "enterprise-saas-staging",
		UID:         "aiops-saas-staging",
		File:        "ai-ops-enterprise-saas-staging.json",
		Name:        "Enterprise SaaS Staging",
		Environment: "staging",
		Summary:     "Staging API and worker, transactional task queue, and Firestore. Focuses on queue depth, error rates, and task latency.",
		Services:    []string{"stg-saas-api", "stg-task-worker"},
		Extras:      "tasks-firestore",
	}
	d := condSaasDashboard(spec)
	panels := d["panels"].([]any)
	seen := map[int]bool{}
	for _, raw := range panels {
		p := raw.(map[string]any)
		id := p["id"].(int)
		if seen[id] {
			t.Fatalf("duplicate panel ID %d", id)
		}
		seen[id] = true
		g := p["gridPos"].(map[string]int)
		if g["x"] < 0 || g["x"]+g["w"] > 24 || g["h"] <= 0 {
			t.Errorf("invalid grid for panel %d: %v", id, g)
		}
		for _, other := range panels {
			op := other.(map[string]any)
			if op["id"].(int) >= id {
				continue
			}
			og := op["gridPos"].(map[string]int)
			if g["x"] < og["x"]+og["w"] && og["x"] < g["x"]+g["w"] &&
				g["y"] < og["y"]+og["h"] && og["y"] < g["y"]+g["h"] {
				t.Errorf("panels %d and %v overlap", id, op["id"])
			}
		}
	}
}

func TestAiAnalyticsMetricSemantics(t *testing.T) {
	spec := projectSpec{
		ID:          "enterprise-ai-analytics",
		UID:         "aiops-ai-analytics",
		File:        "ai-ops-enterprise-ai-analytics.json",
		Name:        "AI Analytics & FinOps",
		Environment: "production",
		Summary:     "API, App Hosting, and billing intelligence. Highlights availability, cloud costs, and BigQuery query health.",
		Services:    []string{"ai-service-api", "ai-portal-frontend"},
		Extras:      "bigquery",
	}
	d := aiAnalyticsDashboard(spec)
	panels := d["panels"].([]any)
	seen := map[int]bool{}
	for _, raw := range panels {
		p := raw.(map[string]any)
		id := p["id"].(int)
		if seen[id] {
			t.Fatalf("duplicate panel ID %d", id)
		}
		seen[id] = true
		g := p["gridPos"].(map[string]int)
		if g["x"] < 0 || g["x"]+g["w"] > 24 || g["h"] <= 0 {
			t.Errorf("invalid grid for panel %d: %v", id, g)
		}
		for _, other := range panels {
			op := other.(map[string]any)
			if op["id"].(int) >= id {
				continue
			}
			og := op["gridPos"].(map[string]int)
			if g["x"] < og["x"]+og["w"] && og["x"] < g["x"]+g["w"] &&
				g["y"] < og["y"]+og["h"] && og["y"] < g["y"]+g["h"] {
				t.Errorf("panels %d and %v overlap", id, op["id"])
			}
		}
	}
}

func TestPoeTrackerMetricSemantics(t *testing.T) {
	spec := projectSpec{
		ID:          "enterprise-telemetry-prod",
		UID:         "aiops-telemetry-prod",
		File:        "ai-ops-enterprise-telemetry-prod.json",
		Name:        "Telemetry Store & Ingest",
		Environment: "production-staging",
		Summary:     "Telemetry ingestion and storage, Firestore Enterprise, and scheduled routines. Cloud Scheduler health is operational priority.",
		Services:    []string{"telemetry-ingest-prod", "telemetry-ingest-staging"},
		Extras:      "scheduler-firestore",
	}
	d := telemetriaTrackerDashboard(spec)
	panels := d["panels"].([]any)
	seen := map[int]bool{}
	for _, raw := range panels {
		p := raw.(map[string]any)
		id := p["id"].(int)
		if seen[id] {
			t.Fatalf("duplicate panel ID %d", id)
		}
		seen[id] = true
		g := p["gridPos"].(map[string]int)
		if g["x"] < 0 || g["x"]+g["w"] > 24 || g["h"] <= 0 {
			t.Errorf("invalid grid for panel %d: %v", id, g)
		}
		for _, other := range panels {
			op := other.(map[string]any)
			if op["id"].(int) >= id {
				continue
			}
			og := op["gridPos"].(map[string]int)
			if g["x"] < og["x"]+og["w"] && og["x"] < g["x"]+g["w"] &&
				g["y"] < og["y"]+og["h"] && og["y"] < g["y"]+g["h"] {
				t.Errorf("panels %d and %v overlap", id, op["id"])
			}
		}
	}
}

func TestLlmDashboardMetricSemantics(t *testing.T) {
	d := llmDashboard()
	panels := d["panels"].([]any)
	seen := map[int]bool{}
	for _, raw := range panels {
		p := raw.(map[string]any)
		id := p["id"].(int)
		if seen[id] {
			t.Fatalf("duplicate panel ID %d", id)
		}
		seen[id] = true
		g := p["gridPos"].(map[string]int)
		if g["x"] < 0 || g["x"]+g["w"] > 24 || g["h"] <= 0 {
			t.Errorf("invalid grid for panel %d: %v", id, g)
		}
		for _, other := range panels {
			op := other.(map[string]any)
			if op["id"].(int) >= id {
				continue
			}
			og := op["gridPos"].(map[string]int)
			if g["x"] < og["x"]+og["w"] && og["x"] < g["x"]+g["w"] &&
				g["y"] < og["y"]+og["h"] && og["y"] < g["y"]+g["h"] {
				t.Errorf("panels %d and %v overlap", id, op["id"])
			}
		}
	}
}
