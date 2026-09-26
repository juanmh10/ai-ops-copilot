# Centralized Observability: Google Cloud Platform & Firebase

This directory contains the declarative observability configuration, dashboard specifications, and alerting policies for unified monitoring across multi-project Google Cloud Platform and Firebase environments.

The architecture designates the primary production project **`enterprise-core-prod`** as the central **Metrics Scope** anchor, consolidating operational telemetry, Grafana dashboards, synthetic uptime checks, and alerting policies across supervised projects.

---

## 📊 Grafana Dashboards for Operators & Autonomous Agents

The provisioning setup under [`grafana-provisioning/`](../grafana-provisioning/) delivers an integrated suite of six dashboards:

- `unified-cloudrun-ops`: Global overview showing multi-project traffic, container autoscaling, Vertex AI token consumption by model, and invoice billing totals.
- `aiops-core-prod`: Core production web services, ingress traffic, 5xx ratios, and p95 request latencies.
- `aiops-saas-staging`: Staging Cloud Run microservices, Cloud Tasks queue depth, and Firestore read/write latency.
- `aiops-ai-analytics`: Production AI services, Firebase App Hosting, BigQuery query execution rates, and billed scan bytes.
- `aiops-telemetry-prod`: Production and staging telemetry ingestion, Cloud Scheduler job execution, and Firestore Enterprise storage.
- `aiops-ai-gateway`: Quota gateway metrics, token usage by model, HTTP status distributions, and structured API errors.

The catalog in [`monitoring/metrics-catalog.json`](metrics-catalog.json) maintains clear separation between low-cardinality human-readable charts and technical high-cardinality dimensions queried on-demand by the autonomous agent. All JSON dashboards are deterministically rendered via:

```bash
go run monitoring/cmd/render-grafana-dashboards/main.go
```

Authentication adheres to Google Application Default Credentials (ADC) using the host environment's native service identity. No static Service Account keys are accepted. BigQuery billing datasources enforce a 50 MiB limit per query to ensure predictable FinOps performance.

---

## 🎨 Dashboard Design Contract

The [Dashboard Design Contract](DASHBOARD-DESIGN.md) specifies the approved layout rules, 24-column grid proportions, visual color palettes, metric semantics, and reproduction guidelines.

All 6 dashboards follow this standardized UX pattern:
- **Global Overview (`unified-cloudrun-ops`):** High-level summary stat cards, multi-project distribution, Vertex AI token comparisons, and billing summaries.
- **Service Dashboards:** Request rates, 5xx errors, p95 latencies, active instance counts, and container CPU/memory saturation.
- **Asynchronous & Scheduled Tasks:** Queue depths, retry counts, and Cloud Scheduler execution statuses.
- **FinOps & Billing:** Multi-currency invoice breakdown, applied billing credits, net exported totals, and data lag freshness indicators.

---

## 1. Metrics Scope Topology & Architecture

Google Cloud Monitoring uses **Metrics Scopes** to aggregate telemetry across projects without replicating data or altering runtime permissions:

```text
                     ┌─────────────────────────────────────────────────────────┐
                     │          SCOPING PROJECT: enterprise-core-prod          │
                     │  - Unified Operational Dashboard (Cloud Run / Services) │
                     │  - Synthetic Uptime Checks (HTTPS / SSL every 5 min)    │
                     │  - Central Alert Policies (Uptime, 5xx > 1%)           │
                     │  - Notification Channel: ops-lead@example.com           │
                     └────────────────────────────┬────────────────────────────┘
                                                  │
             ┌────────────────────┬───────────────┴───────────────┬────────────────────┐
             ▼                    ▼                               ▼                    ▼
   ┌───────────────────┐┌───────────────────┐           ┌───────────────────┐┌───────────────────┐
   │enterprise-saas-stg││enterprise-ai-anal │           │enterprise-tele-prd││enterprise-ai-gtw │
   │• stg-saas-api     ││• ai-service-api   │           │• telemetry-ingest ││• Quota Gateway    │
   │• stg-task-worker  ││• ai-portal-front  │           │• Firestore db     ││• Vertex AI        │
   │• Cloud Tasks      ││• BigQuery billing │           │• Cloud Scheduler  ││• Gemini API       │
   └───────────────────┘└───────────────────┘           └───────────────────┘└───────────────────┘
```

---

## 2. Directory Structure

```text
monitoring/
├── dashboards/
│   └── unified-cloudrun-dashboard.json      # Declarative multi-project dashboard specification
├── policies/
│   ├── uptime-failure-policy.json           # [P1] Alert: Synthetic availability check failure
│   ├── cloudrun-5xx-error-policy.json       # [P2] Alert: HTTP 5xx error ratio > 1%
│   └── scheduler-failure-policy.json        # [P3] Alert: Cloud Scheduler execution failure
├── cmd/
│   └── render-grafana-dashboards/           # Idempotent Go dashboard generator and tests
├── DASHBOARD-DESIGN.md                      # UI/UX layout and metric contract
├── metrics-catalog.json                     # Metric catalog for human dashboards and agent tools
└── README.md                                # Observability architecture guide (this document)
```

---

## 3. Synthetic Uptime Checks

Synthetic availability checks are globally distributed with SSL/TLS certificate verification:

| Check Name | Target URL | Protocol | Period | SSL Validation | Resource Target |
| :--- | :--- | :---: | :---: | :---: | :--- |
| `portal-prod-check` | `https://portal.example.com/` | HTTPS | 5 min | Yes | `projects/enterprise-core-prod/uptimeCheckConfigs/portal-check-demo` |
| `auth-prod-check` | `https://saas.example.com/` | HTTPS | 5 min | Yes | `projects/enterprise-core-prod/uptimeCheckConfigs/auth-check-demo` |
| `telemetry-prod-check` | `https://telemetry.example.com/` | HTTPS | 5 min | Yes | `projects/enterprise-core-prod/uptimeCheckConfigs/telemetry-check-demo` |
| `ai-prod-check` | `https://ai.example.com/` | HTTPS | 5 min | Yes | `projects/enterprise-core-prod/uptimeCheckConfigs/ai-check-demo` |

---

## 4. Alerting Policy Matrix

| Priority | Target Project | Policy Name | Trigger Condition | Evaluation Window | Severity |
| :---: | :--- | :--- | :--- | :---: | :---: |
| **P1** | `enterprise-core-prod` | `[P1] Uptime Check Failure` | Pass rate of any synthetic uptime check $< 1.0$ | 300s (5 min) | `CRITICAL` |
| **P2** | `enterprise-core-prod` | `[P2] Cloud Run - Elevated 5xx Errors` | 5xx error ratio $> 1\%$ of total request volume | 180s (3 min) | `HIGH` |
| **P3** | `enterprise-core-prod` | `[P3] Cloud Scheduler - Job Failure` | Scheduled job execution logs with `severity >= ERROR` | Immediate (Log-based) | `WARNING` |
| **P3** | `enterprise-telemetry-prod` | `[P3] Cloud Scheduler - Job Failure` | Scheduled job execution logs with `severity >= ERROR` in active worker | Immediate (Log-based) | `WARNING` |

---

## 5. FinOps & Free Tier Compliance

The Google Cloud Operations Suite provides an *Always Free Tier* that covers the observability footprint of this architecture:

1. **GCP Metric Ingestion (Cloud Run, Storage, Tasks):** Incur $0.00/month. Standard Google Cloud metrics are ingested without ingestion fees.
2. **Synthetic Uptime Checks:** Free tier provides up to 1,000,000 executions per billing account per month. Four checks running every 5 minutes across 3 regions generate ~103,680 executions/month (~10% of free tier quota).
3. **Cloud Monitoring Dashboards:** Free of charge to create, render, and share.
4. **Log-based Alerts & Ingestion:** First 50 GiB/month of ingested logs per project are free under Cloud Logging. Scheduler job error log volume is negligible (<100 MiB/month).

**Net Compute Cost:** $0.00/month for baseline observability.
