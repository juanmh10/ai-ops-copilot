# Artifact Registry repository to store agent and Grafana container images
resource "google_artifact_registry_repository" "copilot_repo" {
  location      = var.region
  repository_id = "ops-copilot"
  description   = "Docker repository for AI-Ops Copilot container images"
  format        = "DOCKER"
  project       = var.project_id

  depends_on = [
    google_project_service.enabled_apis["artifactregistry.googleapis.com"]
  ]
}
