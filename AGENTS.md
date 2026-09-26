# AGENTS.md — Autonomous Agent Governance, Rules, and Architecture Guidelines

This document defines the governance contract, architectural constraints, and engineering standards for **any AI Agent, assistant, or developer** developing, extending, or maintaining the **AI-Ops Copilot** project.

---

## 1. Project Mission

The **AI-Ops Copilot** is an enterprise-grade autonomous Site Reliability Engineering copilot (*Autonomous SRE*) designed to run as a serverless sidecar microservice coupled to **Grafana OSS** on Google Cloud Platform (GCP).

Its core purpose is to enable an authorized human operator (`ops-lead@example.com`) to query metrics, logs, events, billing summaries, and infrastructure anomalies across interconnected GCP/Firebase projects in a natural conversational flow, enriched with dynamic context from the currently active Grafana dashboard and panel.

---

## 2. Non-Negotiable Rules (Architecture & Safety Guardrails)

Any AI agent generating code, altering configurations, or proposing infrastructure changes **MUST strictly follow these guardrails**:

### 🚫 RULE 1: 100% Read-Only Scope in MVP
* **NO infrastructure write, mutation, or deletion actions are permitted in the MVP.**
* The agent sidecar must provide and invoke **exclusively read-only tools** (`QueryCloudMonitoring`, `QueryCloudLogging`, `GetDashboardMetrics`, `QueryBillingCosts`, `GetFirestoreDigest`).
* Never implement tools that invoke mutating operations such as `gcloud run services update`, `terraform apply`, `delete`, `restart`, or database mutation without explicit human approval.

### 🔑 RULE 2: Zero Static Keys / Hardcoded Credentials
* It is **strictly prohibited** to generate, download, or commit Service Account keys in `.json` or any static secret format.
* All authentication with Google Cloud services must use **Application Default Credentials (ADC)** and federated identities (Workload Identity Federation / Cloud Run native Service Accounts).
* Runtime secrets and configuration passwords must reside in **Google Secret Manager** or be injected via environment variables at runtime.

### 💰 RULE 3: Strict FinOps & Zero Idle Cost
* Every provisioned compute workload must respect the principle of **scale down to zero (`minScale: 0`)**.
* Do not provision continuously running Compute Engine instances (VMs), permanent Cloud SQL instances, or permanent GKE clusters.
* Both Grafana and the Go agent sidecar must sleep when inactive and wake up on demand via incoming HTTP traffic.

### ⚙️ RULE 4: Fixed Technology Stack
* **Agent Backend:** Strictly **Go (Golang 1.23+)**. Do not introduce Python or Node.js runtimes into the agent backend.
* **GenAI SDK:** Official **`google.golang.org/genai`** package configured with **Vertex AI** backend (`genai.BackendVertexAI`).
* **Default Model:** **`gemini-3.8-flash`** in region `us-central1`.
* **Chat Frontend:** React + Grafana Plugin SDK (`@grafana/data`, `@grafana/ui`).
* **State & Persistence:** Cloud Storage (GCS FUSE) for the Grafana SQLite database and Cloud Firestore Native for sessions and telemetry digests.

### 🌐 RULE 5: Absolute Portability, Zero Local Paths & CI/CD Injection
* **Zero Local Absolute Paths:** It is **strictly prohibited** to commit code, scripts, manifests, or documentation containing local filesystem paths (e.g., `/home/...`, `C:\...`). All paths must be **relative to the repository root** or dynamically resolved via environment flags.
* **Zero Secret Exposure:** No runtime secrets, private URLs, or credentials may be hardcoded or committed. All environment parameters are injected via GitHub Actions Environments and Secrets during CI/CD and IaC pipelines.
* **Agnostic Repository:** The repository must be 100% reproducible and agnostic to any developer's local workstation, allowing any clean CI runner to build and test the codebase.

### 🛑 RULE 6: Explicit Human Approval Gate (Commits, Terraform Apply & Destroy)
* **Commits and Push:** NEVER execute `git commit` or `git push` without presenting a summary of changes (`git status` / `git diff`) and receiving explicit authorization from the human operator.
* **Terraform Apply and Destroy:** It is **strictly prohibited** to execute `terraform apply` or `terraform destroy` without presenting the detailed `terraform plan` output and awaiting unambiguous approval from the operator.

### 🔄 RULE 7: Process Liveness & Terminal Anti-Loop Guardrails
* **No Blind Polling:** NEVER get stuck in terminal loops, blocking commands without timeouts, or indefinite sleep loops waiting for background processes that may have already failed.
* **Active Liveness Inspection:** Actively verify process health and container status (`docker ps`, `docker logs`, exit codes) before and during any wait states.
* **Fail-Fast:** If a container crashes, exits with a non-zero exit code, runs out of memory, or deadlocks, abort immediately, gather error diagnostics, and inform the operator rather than waiting indefinitely.

### 👁️ RULE 8: Mandatory Visual UI Validation in Grafana
* **Visual Inspection:** Any modification to dashboards, panels, React plugin components (`@grafana/data`, `@grafana/ui`), or layout styles must be verified visually.
* **UI Verification:** Use automated visual inspection tools (e.g., browser automation, Playwright CLI, or screenshots) to verify that rendered interfaces display correctly without layout regressions or console errors.

### ☁️ RULE 9: Synchronized IaC & GitHub Configuration with Official Cloud Guidance
* **Continuous IaC & CI/CD Sync:** Maintain Terraform manifests and GitHub repository configurations strictly synchronized. Any new infrastructure parameter, variable, or secret must be documented in Terraform (`variables.tf`, `outputs.tf`, `main.tf`) and aligned with GitHub Actions workflows.
* **Adherence to Official Cloud Patterns:** Follow official Google Cloud developer documentation and best practices when designing or querying cloud infrastructure.

---

## 3. Reference Multi-Project Topology

The copilot operates in a host project and monitors interconnected projects through Google Cloud Metrics Scope:

| Project Identifier (Example) | Role / Architecture Domain | Primary Supervised Resources |
| :--- | :--- | :--- |
| **`enterprise-core-prod`** | **Host & Scoping Anchor** | Cloud Run (`portal-api`, `auth-gateway-api`), Terraform State GCS bucket, Secret Manager, Central Cloud Monitoring Metrics Scope. |
| **`enterprise-saas-staging`** | **Staging SaaS Domain** | Cloud Run (`stg-saas-api`, `stg-task-worker`), Cloud Tasks queues, Firestore Native `(default)`, GCS Media storage. |
| **`enterprise-ai-analytics`** | **Production AI & FinOps** | Cloud Run (`ai-service-api`), Firebase App Hosting (`ai-portal-frontend`), BigQuery `billing_export` dataset. |
| **`enterprise-telemetry-prod`** | **Production & Staging Telemetry** | Cloud Run (`telemetry-ingest-prod`, `telemetry-ingest-staging`), Firestore Native `telemetry-store` (Enterprise), Cloud Scheduler jobs. |
| **`enterprise-ai-gateway`** | **Model Quotas Gateway** | Vertex AI quotas and Gemini API throughput (serverless, no permanent compute). |

> [!NOTE]
> Project identifiers above are illustrative reference mappings used in default configuration and test fixtures. All project IDs are configurable via Terraform variables and environment settings.

---

## 4. Software Engineering Standards

### 4.1. Go Codebase (`agent-backend/`)
1. **Standard Directory Layout:**
   - `cmd/server/main.go`: Server entrypoint, dependency injection, and graceful shutdown.
   - `internal/agent/`: Gemini orchestrator, prompt synthesis, dynamic scope resolution, and Function Calling loop.
   - `internal/tools/`: Isolated read-only tool implementations with mockable provider interfaces.
   - `internal/memory/`: Session management and conversation persistence via Cloud Firestore.
2. **Error Handling:** Avoid `panic()` in HTTP runtime. Always propagate and handle errors explicitly with structured logging.
3. **Context Management:** Every outbound call (Google Cloud APIs, Grafana, Gemini) must accept and respect `context.Context` with appropriate deadlines.
4. **Container Image:** Mandatory multi-stage build:
   - Stage 1: `golang:1.23-alpine` (static binary compilation with `CGO_ENABLED=0`).
   - Stage 2: `gcr.io/distroless/static-debian12:nonroot` (final image < 25MB).

### 4.2. Terraform Modules (`terraform/ops-copilot/`)
1. All HCL code must pass `terraform fmt -check` and `terraform validate`.
2. **Human Approval:** Never execute cloud resource creation or destruction without prior operator sign-off.
3. **Zero Secrets in State:** Never commit `.tfvars` or `.tfstate` files containing sensitive credentials.

### 4.3. Git & Commit Conventions
1. **Conventional Commits:** Follow the standard format:
   - `feat: ...` for new capabilities.
   - `fix: ...` for bug fixes.
   - `docs: ...` for documentation and specifications.
   - `refactor: ...` for code restructurings without behavior change.
   - `chore: ...` for tooling, dependency, or CI/CD updates.
2. Always review `git status` and `git diff` before requesting commit approval.

### 4.4. Grafana Plugin (`grafana-plugin/`)
1. **Design System:** Follow [`monitoring/DASHBOARD-DESIGN.md`](monitoring/DASHBOARD-DESIGN.md) for dashboard styling, panel proportions, and metric semantics.
2. All TypeScript components must compile cleanly (`npm run build` and `npm test`).
3. Ensure the plugin remains unobtrusive, injecting the floating copilot drawer cleanly into the Grafana UI without interfering with native dashboard interaction.

---

## 5. Development & Contribution Workflow

1. **Context-First Verification:** Consult official documentation and architecture contracts before planning cloud changes.
2. **Local Emulation First:** Every component must be fully testable locally using Docker Compose, mock providers, and emulators without requiring live GCP credentials.
3. **Active Liveness & Anti-Loop:** Proactively check container health and test outputs to catch regressions early.
4. **Visual & Functional Testing:** Validate UI changes visually and run automated test suites before submitting changes.
5. **Human Approval Gate:** Present proposed plans, code diffs, and critical actions to the human operator before executing changes.
