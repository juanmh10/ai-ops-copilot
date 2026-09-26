# Dedicated Service Account for the Cloud Run runtime (Grafana + Agent)
resource "google_service_account" "ops_copilot" {
  account_id   = "ops-copilot-runtime"
  display_name = "AI-Ops Copilot Runtime Service Account"
  description  = "Execution identity for the Grafana Cloud Run service and Copilot ADK Agent sidecar"
  project      = var.project_id

  depends_on = [
    google_project_service.enabled_apis["iam.googleapis.com"]
  ]
}

# Permissions on the GCS bucket for Grafana persistent data
resource "google_storage_bucket_iam_member" "grafana_storage_admin" {
  bucket = google_storage_bucket.grafana_data.name
  role   = "roles/storage.admin"
  member = "serviceAccount:${google_service_account.ops_copilot.email}"
}

# Permission for the runtime to generate internal credential tokens
resource "google_service_account_iam_member" "sa_token_creator" {
  service_account_id = google_service_account.ops_copilot.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${google_service_account.ops_copilot.email}"
}

# Permission to read and write conversation history and snapshots in Cloud Firestore
resource "google_project_iam_member" "firestore_user" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.ops_copilot.email}"
}

# Read permissions for metrics and dashboards in the central Scoping Project
resource "google_project_iam_member" "monitoring_viewer" {
  project = var.project_id
  role    = "roles/monitoring.viewer"
  member  = "serviceAccount:${google_service_account.ops_copilot.email}"
}

# Cross-project permissions for the agent to inspect logs across all monitored projects
resource "google_project_iam_member" "cross_project_logging_viewer" {
  for_each = toset(var.monitored_projects)
  project  = each.value
  role     = "roles/logging.viewer"
  member   = "serviceAccount:${google_service_account.ops_copilot.email}"
}

# Cross-project permissions to read Cloud Monitoring metrics across all monitored projects
resource "google_project_iam_member" "cross_project_monitoring_viewer" {
  for_each = toset(var.monitored_projects)
  project  = each.value
  role     = "roles/monitoring.viewer"
  member   = "serviceAccount:${google_service_account.ops_copilot.email}"
}

# Strictly read-only access to the FinOps export consumed by Grafana.
# The datasource also defines a per-query limit to contain accidental costs.
resource "google_bigquery_dataset_iam_member" "billing_data_viewer" {
  project    = var.billing_project_id
  dataset_id = trimprefix(var.billing_dataset, "${var.billing_project_id}.")
  role       = "roles/bigquery.dataViewer"
  member     = "serviceAccount:${google_service_account.ops_copilot.email}"
}

resource "google_project_iam_member" "billing_job_user" {
  project = var.billing_project_id
  role    = "roles/bigquery.jobUser"
  member  = "serviceAccount:${google_service_account.ops_copilot.email}"
}

# Allows the runtime to execute BigQuery queries in the quota-consuming project
resource "google_project_iam_member" "billing_service_usage_consumer" {
  project = var.billing_project_id
  role    = "roles/serviceusage.serviceUsageConsumer"
  member  = "serviceAccount:${google_service_account.ops_copilot.email}"
}

# Permission for native Vertex AI calls in the quota project (cross-project)
resource "google_project_iam_member" "vertex_ai_user" {
  project = var.vertex_project_id
  role    = "roles/aiplatform.user"
  member  = "serviceAccount:${google_service_account.ops_copilot.email}"
}

# Public ingress / invoker: allows access to the Grafana web interface (authenticated via Grafana login)
resource "google_cloud_run_v2_service_iam_member" "public_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.copilot.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
