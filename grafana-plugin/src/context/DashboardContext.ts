import type { DashboardContext } from '../types';

const PROJECT_IDS = [
  'enterprise-core-prod',
  'enterprise-saas-staging',
  'enterprise-ai-analytics',
  'enterprise-telemetry-prod',
  'enterprise-ai-gateway',
];

interface DashboardScope {
  scope_level: 'global' | 'project';
  project_id?: string;
  project_ids?: string[];
  environment: string;
  service_names: string[];
}

const DASHBOARD_SCOPES: Record<string, DashboardScope> = {
  'unified-cloudrun-ops': {
    scope_level: 'global',
    project_ids: PROJECT_IDS,
    environment: 'mixed',
    service_names: [],
  },
  'aiops-core-prod': {
    scope_level: 'project',
    project_id: 'enterprise-core-prod',
    environment: 'production',
    service_names: ['portal-api', 'auth-gateway-api'],
  },
  'aiops-saas-staging': {
    scope_level: 'project',
    project_id: 'enterprise-saas-staging',
    environment: 'staging',
    service_names: ['stg-saas-api', 'stg-task-worker'],
  },
  'aiops-ai-analytics': {
    scope_level: 'project',
    project_id: 'enterprise-ai-analytics',
    environment: 'production',
    service_names: ['ai-service-api', 'ai-portal-frontend'],
  },
  'aiops-telemetry-prod': {
    scope_level: 'project',
    project_id: 'enterprise-telemetry-prod',
    environment: 'production-staging',
    service_names: ['telemetry-ingest-prod', 'telemetry-ingest-staging'],
  },
  'aiops-ai-gateway': {
    scope_level: 'project',
    project_id: 'enterprise-ai-gateway',
    environment: 'shared',
    service_names: ['vertex-ai'],
  },
};

function getPanelTitle(panelID?: string): string | undefined {
  if (!panelID || typeof document === 'undefined' || !/^[A-Za-z0-9_-]+$/.test(panelID)) {
    return undefined;
  }

  const panel =
    document.querySelector(`[data-panelid="${panelID}"]`) ||
    document.querySelector(`[data-panel-id="${panelID}"]`);
  if (!panel) {
    return undefined;
  }

  const header =
    panel.querySelector('[data-testid="data-testid Panel header"]') ||
    panel.querySelector('[data-testid="panel-header"]') ||
    panel.querySelector('h2');
  return header?.textContent?.trim() || undefined;
}

/**
 * Captures active dashboard context from the browser DOM and URL query parameters.
 */
export function captureDashboardContext(): DashboardContext {
  if (typeof window === 'undefined') {
    return {};
  }

  const url = new URL(window.location.href);
  const params = url.searchParams;

  // Extract time range
  const from = params.get('from') || 'now-1h';
  const to = params.get('to') || 'now';

  // Extract panel if currently viewing/inspecting a specific panel
  const activePanel = params.get('viewPanel') || params.get('editPanel') || undefined;

  // Extract dashboard UID from pathname (e.g. /d/unified-cloudrun/unified-cloudrun-dashboard)
  const pathParts = url.pathname.split('/');
  let dashboardUID: string | undefined;
  const dIndex = Math.max(pathParts.indexOf('d'), pathParts.indexOf('d-solo'));
  if (dIndex !== -1 && pathParts.length > dIndex + 1) {
    dashboardUID = pathParts[dIndex + 1];
  }

  // Extract filters (template variables like var-project_id=...)
  const activeFilters: Record<string, string> = {};
  params.forEach((value, key) => {
    if (key.startsWith('var-')) {
      const filterKey = key.replace('var-', '');
      activeFilters[filterKey] = value;
    }
  });

  const configuredScope = dashboardUID ? DASHBOARD_SCOPES[dashboardUID] : undefined;
  const filteredProject = activeFilters.project_id;
  const projectID = PROJECT_IDS.includes(filteredProject)
    ? filteredProject
    : configuredScope?.project_id;
  const filteredServices = activeFilters.service_name || activeFilters.service_scope;
  const serviceNames = filteredServices && filteredServices !== 'all'
    ? filteredServices.split(',').map((service) => service.trim()).filter(Boolean)
    : configuredScope?.service_names;

  return {
    dashboard_uid: dashboardUID,
    dashboard_title:
      typeof document !== 'undefined'
        ? document.title.replace(' - Grafana', '').trim()
        : undefined,
    time_range: { from, to },
    active_panel: activePanel,
    active_panel_title: getPanelTitle(activePanel),
    active_filters: Object.keys(activeFilters).length > 0 ? activeFilters : undefined,
    scope_level: configuredScope?.scope_level || (projectID ? 'project' : undefined),
    project_id: projectID,
    project_ids: configuredScope?.project_ids,
    environment: activeFilters.environment || configuredScope?.environment,
    service_names: serviceNames && serviceNames.length > 0 ? serviceNames : undefined,
  };
}
