# Product & Engineering Requirements: AI-Ops Copilot

**Deployment Target:** Google Cloud Platform & Firebase  
**Target Architecture:** Multi-Project Serverless Observability Sidecar  
**Specification Version:** 1.0 (MVP)  

---

## 1. Product Overview & Objectives

The **AI-Ops Copilot** is an autonomous Site Reliability Engineering assistant (*Autonomous SRE Copilot*) integrated directly into **Grafana OSS**. It allows an authorized human operator to inspect, query, analyze, and correlate metrics, logs, billing, and anomaly alerts across a multi-project Google Cloud ecosystem via a context-aware chat drawer embedded into dashboard views.

### Core Objectives:
1. **Unified Observability:** Centralize multi-project metrics and operational telemetry into Grafana dashboards alongside a conversational AI sidecar.
2. **Dynamic Screen Context:** Automatically supply the active dashboard UID, time range, focused panel, and template variables to the AI agent during every query.
3. **Pre-computed State:** Eliminate latency during human inquiries by having background Cloud Scheduler tasks synthesize health digests into Cloud Firestore.
4. **Strict FinOps (Zero Idle Cost):** Run as a serverless Cloud Run workload (`minScale: 0`) that scales down to zero when idle, persisting state to Cloud Storage and Firestore.

---

## 2. Architecture Decisions & Technical Stack

| Layer | Selected Technology | Engineering Rationale |
| :--- | :--- | :--- |
| **Agent Backend** | **Go (Golang 1.23+)** | Ultra-low cold start (~10-20ms), tiny Distroless container image (<25MB), minimal memory footprint (~20MB RSS), and high native concurrency. |
| **GenAI SDK** | **`google.golang.org/genai`** | Official unified Google GenAI SDK with first-class Function Calling support and native Vertex AI backend. |
| **AI Platform** | **Google Cloud Vertex AI** | Enterprise cloud execution with IAM-governed Application Default Credentials (ADC) without static API keys. |
| **Default Model** | **`gemini-3.8-flash`** | High efficiency, low latency, optimized for tool execution and cost-effective multi-turn reasoning. |
| **Frontend Plugin** | **React + Grafana Plugin SDK** | Native Grafana App Plugin providing an unobtrusive floating Chat Drawer. |
| **Dashboard Engine** | **Grafana OSS 11.1** | Containerized Grafana with Google Cloud Monitoring datasource and GCS FUSE persistence. |
| **Chat Memory** | **Cloud Firestore Native** | Serverless NoSQL, zero cost at rest, storing sessions under `agent_sessions/{session_id}/messages`. |
| **State Persistence**| **Cloud Storage (GCS FUSE)** | Bucket `${project_id}-ops-grafana-data` mounted to `/var/lib/grafana` for SQLite and dashboard state. |
| **IaC** | **Terraform (Google-Beta >= 5.30)** | Declarative, modular infrastructure code in `terraform/ops-copilot/`. |

---

## 3. Functional Requirements (100% Read-Only MVP)

### FR01 — Grafana Chat Drawer Widget
* Injects a floating action button into the bottom-right corner of all Grafana dashboards.
* Clicking the button expands an interactive drawer showing previous message history, active context pills, and a query input box.

### FR02 — Dynamic Dashboard Context Capture
With every message submitted, the frontend plugin automatically attaches:
* `dashboard_uid`: Identifier of the active dashboard (e.g., `unified-cloudrun-ops`).
* `dashboard_title`: Human-readable title of the dashboard.
* `time_range`: Selected temporal window (e.g., `now-3h` to `now`).
* `active_panel`: Currently focused panel ID and title.
* `template_variables`: Active filters (`var-project`, `var-service`, etc.).

### FR03 — Read-Only Function Calling Catalog
The Go agent backend executes **strictly read-only tools**:
1. `GetDashboardMetrics(panel_id, time_range)`: Retrieves real-time time-series data from the active Grafana panel.
2. `QueryCloudMonitoring(project_id, metric_type, time_range, ...)`: Queries Google Cloud Monitoring API for metric time series.
3. `QueryCloudLogging(project_id, severity, filter, limit)`: Queries Cloud Logging for recent error logs and exceptions.
4. `QueryBillingCosts(project_id, months)`: Queries BigQuery billing export for monthly cost trends and credits.
5. `GetFirestoreDigest(lookback_hours)`: Retrieves pre-computed operational digests compiled by Cloud Scheduler.

> [!CAUTION]
> **Safety Guardrail:** Any mutating, scaling, updating, or deleting tool calls are rejected at the executor level.

### FR04 — Conversational Memory Persistence
* Messages persist in Cloud Firestore under verified user sessions.
* Reopening the drawer restores recent conversation history without requiring full page reload.

### FR05 — Scheduled Background Digest
* A Cloud Scheduler job invokes `/api/digest` every 2 hours via OIDC authenticated requests.
* Compiles high-level project health summaries into Firestore `infra_snapshots`.

---

## 4. Non-Functional Requirements

* **NFR01 — Performance & Latency:** Tool execution loop must maintain a p95 latency under 3.5 seconds when using Vertex AI `gemini-3.8-flash`.
* **NFR02 — Zero-Trust Security:** Cloud Run ingress rejects unauthenticated requests (`--no-allow-unauthenticated`). Access is strictly mediated via Google IAM or Identity-Aware Proxy (IAP).
* **NFR03 — Strict FinOps:** Idle workloads incur zero compute cost via `minScale: 0`. BigQuery billing queries enforce partition filtering to minimize byte scan costs.
* **NFR04 — Portability & Zero Static Secrets:** No `.json` service account keys, passwords, or local paths in version control. All configuration is injected via runtime environment variables and Workload Identity Federation.
* **NFR05 — Reliability & Fail-Fast:** Ephemeral containers gracefully restart within seconds. Agent backend implements strict timeouts and context cancellation on all outbound requests.
