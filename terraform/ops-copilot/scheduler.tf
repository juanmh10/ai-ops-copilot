# Dedicated Service Account for the Cloud Scheduler trigger
resource "google_service_account" "scheduler_invoker" {
  account_id   = "ops-scheduler-invoker"
  display_name = "AI-Ops Cloud Scheduler Invoker"
  project      = var.project_id
}

# Permission for Cloud Scheduler to invoke Cloud Run via OIDC
resource "google_cloud_run_v2_service_iam_member" "scheduler_run_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.copilot.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.scheduler_invoker.email}"
}

# Cloud Scheduler job for periodic collection and pre-digestion of telemetry in Firestore
resource "google_cloud_scheduler_job" "telemetry_digest" {
  name             = "ai-ops-telemetry-digest"
  description      = "Periodically triggers the agent to compile and save telemetry digests in Firestore"
  schedule         = "0 */2 * * *" # Every 2 hours
  time_zone        = "Etc/UTC"
  attempt_deadline = "300s"
  project          = var.project_id
  region           = var.region

  retry_config {
    retry_count = 2
  }

  depends_on = [
    google_project_service.enabled_apis["cloudscheduler.googleapis.com"]
  ]

  http_target {
    http_method = "POST"
    uri         = "${google_cloud_run_v2_service.copilot.uri}/api/agent/digest"

    oidc_token {
      service_account_email = google_service_account.scheduler_invoker.email
      audience              = google_cloud_run_v2_service.copilot.uri
    }

    headers = {
      "Content-Type" = "application/json"
    }

    body = base64encode(jsonencode({
      trigger        = "cloud-scheduler"
      lookback_hours = 2
    }))
  }
}
