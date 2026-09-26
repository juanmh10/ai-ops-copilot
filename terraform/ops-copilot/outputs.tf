output "cloud_run_service_name" {
  description = "Name of the provisioned Cloud Run service"
  value       = google_cloud_run_v2_service.copilot.name
}

output "cloud_run_url" {
  description = "Primary URL for Grafana and AI-Ops Copilot"
  value       = google_cloud_run_v2_service.copilot.uri
}

output "grafana_storage_bucket" {
  description = "Name of the GCS bucket for persistent Grafana data"
  value       = google_storage_bucket.grafana_data.name
}

output "runtime_service_account" {
  description = "Email of the Cloud Run runtime Service Account"
  value       = google_service_account.ops_copilot.email
}

output "scheduler_job_name" {
  description = "Name of the Cloud Scheduler job configured for telemetry pre-digestion"
  value       = google_cloud_scheduler_job.telemetry_digest.name
}

output "github_workload_identity_provider" {
  description = "Workload Identity Provider resource name for GitHub Actions"
  value       = google_iam_workload_identity_pool_provider.github_main.name
}

output "github_cd_service_account" {
  description = "Continuous Deployment Service Account email for GitHub Actions"
  value       = google_service_account.github_deployer.email
}
