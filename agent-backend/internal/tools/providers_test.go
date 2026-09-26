package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealGrafanaClientQueriesTheActivePanel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "grafana_session=valid" {
			t.Errorf("expected Grafana session cookie, got %q", r.Header.Get("Cookie"))
		}
		switch r.URL.Path {
		case "/api/dashboards/uid/aiops-core-prod":
			if r.Method != http.MethodGet {
				t.Errorf("expected dashboard GET, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"dashboard":{"panels":[{"id":12,"title":"Requests","datasource":{"uid":"gcp-monitoring","type":"stackdriver"},"targets":[{"refId":"A","expr":"requests"},{"refId":"B","hide":true}]}]}}`))
		case "/api/ds/query":
			if r.Method != http.MethodPost {
				t.Errorf("expected query POST, got %s", r.Method)
			}
			var body struct {
				From    string           `json:"from"`
				To      string           `json:"to"`
				Queries []map[string]any `json:"queries"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode query body: %v", err)
			}
			if body.From != "now-6h" || body.To != "now" || len(body.Queries) != 1 {
				t.Errorf("query did not preserve the visible range or target: %+v", body)
			}
			if body.Queries[0]["datasource"] == nil {
				t.Errorf("expected panel datasource on the query target: %+v", body.Queries[0])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":{"A":{"frames":[{"schema":{"name":"Requests"},"data":{"values":[[1,2]]}}]}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewRealGrafanaClient(server.URL)
	result, err := client.GetPanelMetrics(context.Background(), DashboardQueryContext{
		DashboardUID: "aiops-core-prod", TimeFrom: "now-6h", TimeTo: "now",
		Cookie: "grafana_session=valid",
	}, "12", "selected")
	if err != nil {
		t.Fatalf("query panel: %v", err)
	}
	if result.PanelID != "12" || result.Dashboard != "aiops-core-prod" || result.Status != "ok" {
		t.Fatalf("unexpected Grafana result: %+v", result)
	}
	if result.Data["A"] == nil {
		t.Fatalf("expected live query results to be returned, got %+v", result.Data)
	}
}

func TestRealGrafanaClientRequiresAnAuthenticatedSession(t *testing.T) {
	client := NewRealGrafanaClient("http://localhost")
	_, err := client.GetPanelMetrics(context.Background(), DashboardQueryContext{DashboardUID: "aiops-core-prod"}, "12", "selected")
	if err == nil || err.Error() != "authenticated Grafana session is required" {
		t.Fatalf("expected authentication requirement, got %v", err)
	}
}
