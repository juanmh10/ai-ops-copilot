package agent

import "testing"

func TestResolveQueryScope(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		context     *DashboardContext
		wantMode    string
		wantSource  string
		wantProject string
	}{
		{
			name:     "uses dashboard project by default",
			message:  "como está a latência?",
			context:  &DashboardContext{ProjectID: "enterprise-saas-staging", ScopeLevel: "project"},
			wantMode: "project", wantSource: "dashboard", wantProject: "enterprise-saas-staging",
		},
		{
			name:     "explicit project overrides dashboard",
			message:  "compare com enterprise-telemetry-prod",
			context:  &DashboardContext{ProjectID: "enterprise-saas-staging", ScopeLevel: "project"},
			wantMode: "project", wantSource: "explicit-message", wantProject: "enterprise-telemetry-prod",
		},
		{
			name:     "global intent overrides dashboard",
			message:  "quero uma visão geral de todos os projetos",
			context:  &DashboardContext{ProjectID: "enterprise-ai-analytics", ScopeLevel: "project"},
			wantMode: "global", wantSource: "explicit-message", wantProject: "enterprise-core-prod",
		},
		{
			name:     "global dashboard is inherited",
			message:  "há algo anormal?",
			context:  &DashboardContext{ScopeLevel: "global"},
			wantMode: "global", wantSource: "dashboard", wantProject: "enterprise-core-prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveQueryScope(tt.message, tt.context)
			if got.Mode != tt.wantMode || got.Source != tt.wantSource || got.EffectiveProject != tt.wantProject {
				t.Fatalf("unexpected scope: %#v", got)
			}
		})
	}
}
