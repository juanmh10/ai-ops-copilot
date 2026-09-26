import test from 'node:test';
import assert from 'node:assert/strict';
import { captureDashboardContext } from './DashboardContext.ts';
import type { ChatRequest, ChatResponse, ChatMessage } from '../types.ts';

test('captureDashboardContext returns empty object when window is undefined', () => {
  const originalWindow = globalThis.window;
  try {
    // @ts-ignore
    delete globalThis.window;
    const ctx = captureDashboardContext();
    assert.deepStrictEqual(ctx, {});
  } finally {
    globalThis.window = originalWindow;
  }
});

test('captureDashboardContext extracts UID, panel, filters, time range and cleans title', () => {
  const mockUrl = 'http://localhost:3000/d/telemetry-store/telemetry-store?from=now-6h&to=now&viewPanel=req-rate&var-project_id=enterprise-telemetry-prod&var-env=production';
  
  // Setup mock DOM globals
  const mockLocation = new URL(mockUrl);
  // @ts-ignore
  globalThis.window = {
    location: mockLocation,
  };
  // @ts-ignore
  globalThis.document = {
    title: 'Telemetry Store Monitoring - Grafana',
    querySelector: () => null,
  };

  try {
    const ctx = captureDashboardContext();

    assert.strictEqual(ctx.dashboard_uid, 'telemetry-store');
    assert.strictEqual(ctx.dashboard_title, 'Telemetry Store Monitoring');
    assert.strictEqual(ctx.active_panel, 'req-rate');
    assert.strictEqual(ctx.project_id, 'enterprise-telemetry-prod');
    assert.strictEqual(ctx.scope_level, 'project');
    assert.deepStrictEqual(ctx.time_range, { from: 'now-6h', to: 'now' });
    assert.deepStrictEqual(ctx.active_filters, {
      project_id: 'enterprise-telemetry-prod',
      env: 'production',
    });
  } finally {
    // @ts-ignore
    delete globalThis.window;
    // @ts-ignore
    delete globalThis.document;
  }
});

test('captureDashboardContext applies sensible defaults when params are missing', () => {
  const mockUrl = 'http://localhost:3000/d/simple-dash';
  
  // @ts-ignore
  globalThis.window = {
    location: new URL(mockUrl),
  };
  // @ts-ignore
  globalThis.document = {
    title: 'Simple Dashboard',
    querySelector: () => null,
  };

  try {
    const ctx = captureDashboardContext();

    assert.strictEqual(ctx.dashboard_uid, 'simple-dash');
    assert.strictEqual(ctx.dashboard_title, 'Simple Dashboard');
    assert.strictEqual(ctx.active_panel, undefined);
    assert.deepStrictEqual(ctx.time_range, { from: 'now-1h', to: 'now' });
    assert.strictEqual(ctx.active_filters, undefined);
  } finally {
    // @ts-ignore
    delete globalThis.window;
    // @ts-ignore
    delete globalThis.document;
  }
});

test('captureDashboardContext derives project scope from provisioned dashboard UID', () => {
  // @ts-ignore
  globalThis.window = {
    location: new URL('http://localhost:3000/d/aiops-saas-staging/project?from=now-3h&to=now'),
  };
  // @ts-ignore
  globalThis.document = {
    title: 'Project — SaaS Staging - Grafana',
    querySelector: () => null,
  };

  try {
    const ctx = captureDashboardContext();

    assert.strictEqual(ctx.project_id, 'enterprise-saas-staging');
    assert.strictEqual(ctx.environment, 'staging');
    assert.deepStrictEqual(ctx.service_names, ['stg-saas-api', 'stg-task-worker']);
    assert.strictEqual(ctx.scope_level, 'project');
  } finally {
    // @ts-ignore
    delete globalThis.window;
    // @ts-ignore
    delete globalThis.document;
  }
});

test('ChatRequest and ChatResponse data models adhere to specification', () => {
  const req: ChatRequest = {
    user_id: 'ops-lead@example.com',
    session_id: 'sess-123',
    message: 'What is the error rate for enterprise-saas-staging?',
    context: {
      dashboard_uid: 'enterprise-saas-staging-stg',
      time_range: { from: 'now-1h', to: 'now' },
    },
  };

  assert.strictEqual(req.user_id, 'ops-lead@example.com');
  assert.strictEqual(req.session_id, 'sess-123');

  const res: ChatResponse = {
    response: 'Service stg-saas-api operating with 0% error rate.',
    session_id: 'sess-123',
    tool_calls: [
      {
        name: 'QueryCloudMonitoring',
        args: { project_id: 'enterprise-saas-staging', metric_type: 'run.googleapis.com/request_count' },
      },
    ],
  };

  assert.strictEqual(res.session_id, 'sess-123');
  assert.strictEqual(res.tool_calls?.length, 1);
  assert.strictEqual(res.tool_calls?.[0].name, 'QueryCloudMonitoring');
});
