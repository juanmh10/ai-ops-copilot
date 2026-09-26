# Programmatic enablement of core GCP APIs
resource "google_project_service" "enabled_apis" {
  for_each = toset([
    "run.googleapis.com",              # Cloud Run Admin API
    "cloudscheduler.googleapis.com",   # Cloud Scheduler API
    "firestore.googleapis.com",        # Cloud Firestore API
    "aiplatform.googleapis.com",       # Vertex AI / Gemini API
    "iam.googleapis.com",              # Identity and Access Management (IAM)
    "iamcredentials.googleapis.com",   # Temporary credentials for federated identity
    "sts.googleapis.com",              # Security Token Service for GitHub OIDC
    "artifactregistry.googleapis.com", # Artifact Registry for images
    "secretmanager.googleapis.com"     # Secret Manager for secrets
  ])

  project            = var.project_id
  service            = each.key
  disable_on_destroy = false
}
