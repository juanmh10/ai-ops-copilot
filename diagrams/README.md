# GCP & Firebase Architecture Diagrams

[Editable Diagram](infra-gcp-firebase.drawio) · [Vector Preview](infra-gcp-firebase.svg)

This directory contains visual architecture diagrams representing the multi-project Google Cloud Platform and Firebase infrastructure topology monitored by **AI-Ops Copilot**.

---

## 📐 Scope & Visual Elements

The diagram captures the reference ecosystem:
- **Projects & Environments:** Scoping Anchor (`enterprise-core-prod`), SaaS Staging (`enterprise-saas-staging`), AI & Analytics (`enterprise-ai-analytics`), Telemetry Ingestion (`enterprise-telemetry-prod`), and Quota Gateway (`enterprise-ai-gateway`).
- **Compute Services:** Cloud Run serverless microservices, Firebase App Hosting, and Cloud Functions.
- **Data & Storage:** Cloud Storage buckets, Cloud Firestore Native databases, and BigQuery datasets.
- **Asynchronous Execution:** Cloud Tasks queues and Cloud Scheduler jobs.
- **Security & Delivery:** Workload Identity Federation (WIF) pools and least-privilege service identities.

Blocks can be edited and explored inside [diagrams.net (draw.io)](https://app.diagrams.net/). Metadata and tooltips register resource capacities, runtime identities, and configuration paths.

---

## 🔍 Reading Guidelines

- **Solid Line:** Documented functional relationship or end-user HTTP access.
- **Dashed Line:** Governance, identity assertion, or deployment delivery.
- **Dotted Line:** Inferred logical dependency between service components.
- **Proximity:** Visual grouping indicates project ownership rather than local network attachment.

---

## ⚡ Cloud Run Service Sizing (Reference Topology)

All microservices reside in `us-central1` with `minScale: 0` (zero idle compute cost) and concurrency configured to 80:

| Project | Service | vCPU | Memory | Max Instances | Execution Identity |
| :--- | :--- | :---: | :---: | :---: | :--- |
| `enterprise-core-prod` | `portal-api` | 1 | 512 MiB | 20 | `portal-api-runtime` |
| `enterprise-core-prod` | `auth-gateway-api` | 1 | 256 MiB | 1 | `auth-gateway-runtime` |
| `enterprise-ai-analytics` | `ai-service-api` | 1 | 512 MiB | 1 | `ai-service-runtime` |
| `enterprise-ai-analytics` | `ai-portal-frontend` | 1 | 512 MiB | 100 | `firebase-app-hosting-compute` |
| `enterprise-telemetry-prod` | `telemetry-ingest-prod` | 1 | 256 MiB | 1 | `telemetry-runtime` |
| `enterprise-telemetry-prod` | `telemetry-ingest-staging`| 1 | 256 MiB | 1 | `telemetry-runtime` |
| `enterprise-saas-staging` | `stg-saas-api` | 1 | 512 MiB | 2 | `stg-auth-runtime` |
| `enterprise-saas-staging` | `stg-task-worker` | 1 | 512 MiB | 1 | `stg-task-worker` |
