# AI-Ops Dashboard Design Contract

**Status:** Design Contract & UX Standard  
**Target Platform:** Grafana OSS 11.1+  
**Design Reference:** Global Overview (`unified-cloudrun-ops`) and Project Dashboards  

---

## 1. Reference & Scope

The reference implementation is defined in:
- [Go Dashboard Generator](cmd/render-grafana-dashboards/main.go): `overviewDashboard`, `overviewPanel`, `overviewRow`, and `overviewBillingSQL`.
- [Provisioned JSON Dashboards](../grafana-provisioning/dashboards/json/).
- [Generator Test Suite](cmd/render-grafana-dashboards/main_test.go).

All dashboards in the observability suite adhere to this unified design contract:
1. `unified-cloudrun-ops`: Global ecosystem overview (traffic, 5xx errors, AI tokens, and multi-currency billing).
2. `aiops-core-prod`: Core production web services, ingress traffic, 5xx ratios, and p95 latency.
3. `aiops-saas-staging`: Staging microservices, Cloud Tasks queue depth, and Firestore health.
4. `aiops-ai-analytics`: Production AI services, BigQuery billing dataset query rates, and scan volume.
5. `aiops-telemetry-prod`: Production & staging telemetry ingestion, Cloud Scheduler jobs, and Firestore storage.
6. `aiops-ai-gateway`: Quota gateway metrics, token usage distribution by model, and HTTP response categories.

---

## 2. Operator Reading Order

Every dashboard must be structured to answer the following operational questions in strict hierarchical order:

1. **Volume & Impact:** What was the overall traffic volume or impact during this period?
2. **Failure & Latency:** Where are the traffic spikes, error crashes, and latency regressions?
3. **Resource Saturation:** Which specific project resource (CPU, memory, queue, scheduler) requires immediate attention?
4. **FinOps & Cost:** How much did this cost, and what is the data freshness of the billing export?

Every panel must answer a direct operational question. Avoid decorative gauges, meaningless static numbers, or uninterpretable metrics.

---

## 3. Grid Layout & Proportions

Dashboards use Grafana's native **24-column grid**. Heights are expressed in `gridPos.h` units:

| Element Type | Width (`w`) | Height (`h`) | Layout Rule |
| :--- | :---: | :---: | :--- |
| **Intro Banner** | 24 | 3 | Text panel, Markdown H2 title and short context paragraph |
| **KPI Stat Row** | 6 × 4 | 4 | Four KPI stat cards at positions x=0, 6, 12, 18 |
| **Section Row Divider** | 24 | 1 | Collapsible row, expanded by default |
| **Paired Time Series** | 12 × 2 | 8 | Equal height side-by-side comparison |
| **Trio Comparison** | 8 × 3 | 9 | Horizontal bar gauge, donut breakdown, and vertical bar chart |
| **Billing Invoice Summary** | 18 | 5 | Compact table, explicit currency columns |
| **Billing Freshness Lag** | 6 | 5 | KPI stat panel placed alongside billing table |
| **Cost Detail & History** | 12 × 2 | 9 | Table and 12-month vertical history bar chart |
| **Interpretation Note** | 24 | 3 | Transparent text panel, footer notes |

---

## 4. Typography & Label Conventions

- Use clear, professional, recognizable titles.
- Use `" · "` to separate the metric name from the scope or segmentation.
- Preserve canonical model names (e.g., `gemini-3.8-flash`, `gemini-1.5-pro`).

### Approved Section Structure:
- `01  Operations · Traffic, Error Ratios, and Latency`
- `02  Resources & Processing · Queues, Jobs, and Saturation`
- `03  Costs & Billing · Invoice Breakdown and Data Freshness`

### Labeling Standards:

| Operational Context | Recommended Label | Avoid |
| :--- | :--- | :--- |
| Event Totals | `Requests · Period Total` | `Current Requests` (when representing 24h sum) |
| Error Counts | `5xx Errors · Period Total` | `Error Rate` (when no denominator is present) |
| Traffic Series | `Traffic by Service` | Raw GCP metric string (`run.googleapis.com/...`) |
| AI Consumption | `Tokens · Input & Output` | `Total/Total` or ambiguous acronyms |
| Model Breakdown | `Model Invocations · Share` | `AI Usage` |
| Historical Costs | `Latest Available Invoice Summary`| `Current Cost` (when export has lag) |
| Missing Data | `No data` | `0`, `Healthy`, `Available` |

Every data panel must include a `description` tooltip detailing: measurement type, unit, aggregation method, scope, and interpretation boundaries.

---

## 5. Visual Palette & Panel Configurations

Use Grafana's standard semantic palette:

| Element | Color |
| :--- | :--- |
| Requests Stat | `blue` |
| 5xx Errors Stat | `orange` |
| AI Tokens Stat | `purple` |
| AI Invocations Stat | `cyan` |
| Core Production Series | `blue` |
| SaaS Staging Series | `orange` |
| AI & Analytics Series | `purple` |
| Telemetry Ingestion Series | `green` |
| Input / Output Tokens | `purple` / `cyan` |
| Null / Missing Values | `gray` |
| Model Identities | `palette-classic` (consistent order across panels) |

Categorical colors do not denote health: green in a time series simply identifies a specific project or model. Real alerts require explicit thresholds and notifications.

### Stat Panels:
- `orientation=horizontal`, `textMode=value`, `colorMode=value`, `graphMode=none`, `justifyMode=left`.
- Counts use unit `short` with 0 decimal places.
- Freshness lag calculates `MAX(export_time)` from billing data, displayed with `suffix: days` and labeled `Data Lag`.

### Time Series:
- Line width 2px, interpolation `linear`, fill opacity 8%, points `never`, `spanNulls=false`.
- Legend in table list format at footer without calculations, tooltip `multi` sorted `desc`.
- `maxDataPoints=1500`.

### Total Gauges (Horizontal Bars):
- `bargauge`, horizontal, `displayMode=basic`, `showUnfilled=false`, `valueMode=color`.
- Calculate sums using `reduce/reduceFields/sum` transformation **before** deriving scale.

### Model Breakdown (Donut):
- `piechart` with donut hole, percentage displayed on slices, legend table at footer.
- Reduction `sum`. Use mutually exclusive categories.

### Billing Tables & Bar Charts:
- Tables: visible headers, `cellHeight=md`, no auto-sum footer. Explicit columns for Project, Currency, Gross, Credits, and Net.
- Monthly history: vertical bar chart, `groupWidth=0.7`, `barWidth=0.8`, fill opacity 85%, labels angled at −45°.

---

## 6. Metric Semantics & Guardrails

| Question | Correct Aggregation Operation | Unit |
| :--- | :--- | :--- |
| How many events occurred? | DELTA: `ALIGN_SUM`, `REDUCE_SUM` summed over the period | `short` |
| What is the traffic rate? | DELTA: `ALIGN_RATE` summed across relevant series | `reqps` or `ops` |
| How many active instances? | GAUGE: time-averaged within interval, summed across series | `short` |
| What is the worst p95 latency? | `ALIGN_PERCENTILE_95` per series, `REDUCE_MAX` across series | Milliseconds (`ms`) |
| Which model received the most calls? | `model_invocation_count` grouped by `model_user_id`, summed | Invocations / % |
| Which model consumed the most tokens? | `token_count` grouped by `model_user_id`, summed | Tokens |
| What was the net invoice cost? | Gross + signed credits, grouped by currency and invoice | Currency |

### Rules:
- Never sum rate-per-second values to invent total counts; use DELTA metric counters.
- Never use `lastNotNull` to represent a full period total.
- Percentile of percentiles is not a global percentile. Always clarify when displaying the maximum of p95 series.
- Configure `noValue="No data"`; do not replace missing data with zero.

---

## 7. Adaptation by Project Scope

### Core Production (`aiops-core-prod`)
- Focus: Production Cloud Run APIs, ingress traffic, 5xx error ratios, and p95 latency.
- Section 01: Service traffic and error breakdown.
- Section 02: Container CPU and memory saturation ($p99$).
- Section 03: Project billing history.

### SaaS Staging (`aiops-saas-staging`)
- Focus: Staging APIs, background Cloud Tasks queues, and Cloud Firestore latency.
- Section 01: Ingress traffic and error ratios.
- Section 02: Queue depth, task dispatch attempts, and Firestore read/write latency.
- Section 03: Project billing history.

### AI & Analytics (`aiops-ai-analytics`)
- Focus: Production AI services, Firebase App Hosting, BigQuery query execution, and scanned bytes.
- Section 01: Service traffic and latency.
- Section 02: BigQuery queries/sec and billed bytes scanned.
- Section 03: Project billing history.

### Telemetry Ingestion (`aiops-telemetry-prod`)
- Focus: Ingestion Cloud Run services, Cloud Scheduler jobs, and Firestore Enterprise storage.
- Section 01: Production vs Staging ingestion traffic and 5xx errors.
- Section 02: Cloud Scheduler job failures, Firestore operations, and document storage.
- Section 03: Project billing history.

### AI Model Gateway (`aiops-ai-gateway`)
- Focus: Vertex AI model quotas, token throughput, and HTTP response distributions (serverless, no permanent compute).
- Section 01: Token consumption by model (input vs output).
- Section 02: Request throughput, HTTP error codes, and structured failure categories.
- Section 03: Gateway billing history.

---

## 8. Workflow for Autonomous Agents & Contributors

1. Review [`AGENTS.md`](../AGENTS.md), this design contract, and [`metrics-catalog.json`](metrics-catalog.json).
2. Identify the target dashboard UID and the specific questions each panel must address.
3. Implement modifications in the Go dashboard generator (`cmd/render-grafana-dashboards/main.go`).
4. Regenerate provisioned JSON dashboards and run tests:
   ```bash
   go run monitoring/cmd/render-grafana-dashboards/main.go
   go test -v ./...
   ```
5. Inspect the rendered dashboards visually in Grafana (1280px and 1600px widths).
6. Verify that no panels overlap, units are clean, and no private identifiers are exposed.
