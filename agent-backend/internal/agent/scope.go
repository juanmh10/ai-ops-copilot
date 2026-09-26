package agent

import "strings"

var monitoredProjects = []string{
	"enterprise-core-prod",
	"enterprise-saas-staging",
	"enterprise-ai-analytics",
	"enterprise-telemetry-prod",
	"enterprise-ai-gateway",
}

var projectAliases = []struct {
	ProjectID string
	Aliases   []string
}{
	{ProjectID: "enterprise-core-prod", Aliases: []string{"core-prod", "core", "portal", "production portal", "auth-gateway"}},
	{ProjectID: "enterprise-saas-staging", Aliases: []string{"enterprise-saas-staging", "saas", "staging saas", "task-worker"}},
	{ProjectID: "enterprise-ai-analytics", Aliases: []string{"enterprise-ai-analytics", "ai-analytics", "analytics", "ai-service"}},
	{ProjectID: "enterprise-telemetry-prod", Aliases: []string{"enterprise-telemetry-prod", "telemetry-prod", "telemetry", "telemetry-store"}},
	{ProjectID: "enterprise-ai-gateway", Aliases: []string{"enterprise-ai-gateway", "ai-gateway", "llm-api", "vertex", "gemini"}},
}

// ScopeResolution explains which project scope should be used for the current
// question and why. Explicit intent in the message always wins over the active
// dashboard, followed by global intent and then the dashboard context.
type ScopeResolution struct {
	Mode             string   `json:"mode"`
	Source           string   `json:"source"`
	EffectiveProject string   `json:"effective_project,omitempty"`
	ProjectIDs       []string `json:"project_ids,omitempty"`
}

// ResolveQueryScope determines whether a question is global or project-scoped.
func ResolveQueryScope(message string, dashboard *DashboardContext) ScopeResolution {
	normalized := strings.ToLower(strings.TrimSpace(message))

	for _, projectID := range monitoredProjects {
		if strings.Contains(normalized, strings.ToLower(projectID)) {
			return projectScope(projectID, "explicit-message")
		}
	}
	for _, entry := range projectAliases {
		for _, alias := range entry.Aliases {
			if strings.Contains(normalized, alias) {
				return projectScope(entry.ProjectID, "explicit-message")
			}
		}
	}

	globalTerms := []string{
		"all projects", "overview", "global", "ecosystem", "consolidated", "all services",
		"todos os projetos", "todos projetos", "visão geral", "visao geral", "ecossistema", "consolidado",
	}
	for _, term := range globalTerms {
		if strings.Contains(normalized, term) {
			return globalScope("explicit-message")
		}
	}

	if dashboard != nil {
		if isMonitoredProject(dashboard.ProjectID) {
			return projectScope(dashboard.ProjectID, "dashboard")
		}
		if dashboard.ScopeLevel == "global" {
			return globalScope("dashboard")
		}
	}

	return globalScope("default")
}

func projectScope(projectID, source string) ScopeResolution {
	return ScopeResolution{
		Mode:             "project",
		Source:           source,
		EffectiveProject: projectID,
		ProjectIDs:       []string{projectID},
	}
}

func globalScope(source string) ScopeResolution {
	projects := make([]string, len(monitoredProjects))
	copy(projects, monitoredProjects)
	return ScopeResolution{
		Mode:             "global",
		Source:           source,
		EffectiveProject: "enterprise-core-prod",
		ProjectIDs:       projects,
	}
}

func isMonitoredProject(projectID string) bool {
	for _, allowed := range monitoredProjects {
		if projectID == allowed {
			return true
		}
	}
	return false
}
