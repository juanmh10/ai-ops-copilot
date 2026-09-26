# Terraform Module: AI-Ops Copilot & Serverless Grafana

This Terraform module provisions the complete infrastructure for **AI-Ops Copilot** on Google Cloud Platform, deploying a multi-container serverless sidecar on Cloud Run v2 with persistence in Cloud Storage (GCS FUSE) and session memory in Cloud Firestore.

---

## 📦 Provisioned Resources

1. **Cloud Storage Bucket (`google_storage_bucket`):**
   - `${project_id}-ops-grafana-data` (Mounted to `/var/lib/grafana` via GCS FUSE).
   - Object versioning enabled to protect the Grafana SQLite database (`grafana.db`) and dashboard files.
2. **Service Accounts & Least-Privilege IAM (`google_service_account`):**
   - `ops-copilot-runtime`: Read-only permissions (`roles/monitoring.viewer`, `roles/logging.viewer` across all supervised projects, `roles/datastore.user`, `roles/storage.objectAdmin`, `roles/bigquery.jobUser`, and `roles/bigquery.dataViewer` for billing exports).
   - `ops-scheduler-invoker`: Dedicated invoker identity with `roles/run.invoker` for automated digest generation.
3. **Multi-Container Cloud Run v2 Service (`google_cloud_run_v2_service`):**
   - Container 1: `grafana` (Port 3000, pre-installed Google Cloud Monitoring plugin, GCS FUSE volume).
   - Container 2: `agent-backend` (Port 8080, Go 1.23, Vertex AI GenAI SDK sidecar).
   - Autoscaling: `min_instance_count = 0` (Zero idle cost), `max_instance_count = 1`.
   - Access: Authenticated ingress restricted to the authorized operator (`authorized_user_email`).
4. **Cloud Scheduler Job (`google_cloud_scheduler_job`):**
   - Dispatches an OIDC-authenticated HTTP request to `/api/digest` every 2 hours to compile telemetry digests in Firestore.
5. **Workload Identity Federation (`google_iam_workload_identity_pool`):**
   - Keyless GitHub Actions OIDC integration for automated image builds and CD deployments.

---

## 🚀 Local Validation & Planning Workflow

```bash
cd terraform/ops-copilot

# 1. Copy sample configuration
cp terraform.tfvars.example terraform.tfvars

# 2. Initialize provider plugins
terraform init

# 3. Format and validate HCL syntax
terraform fmt -check
terraform validate

# 4. Dry-run planning
terraform plan
```

> [!CAUTION]
> As defined in [`AGENTS.md`](../../AGENTS.md), `terraform apply` and `terraform destroy` must never be executed without explicit operator review and approval.

---

## 🔄 Continuous Deployment (CD) via Workload Identity Federation

The deployment pipeline in `.github/workflows/ci.yml` builds and deploys upon merge to `main`:
1. Requires all CI test and security audit jobs to pass.
2. Builds Distroless container images for `agent-backend` and `grafana`.
3. Pushes images to Google Artifact Registry tagged with the commit SHA.
4. Deploys a new revision to Cloud Run v2 with immutable image digests.
5. Employs Workload Identity Federation (WIF) with zero static service account keys.
