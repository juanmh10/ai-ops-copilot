# Secret Manager for secure credentials and log sanitization
# Prevents exposure of plaintext passwords and keys in Cloud Run environment variables

# 1. Grafana administrator password secret
resource "google_secret_manager_secret" "grafana_admin_password" {
  secret_id = "grafana-admin-password"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = {
    managed-by = "terraform"
    service    = "ai-ops-copilot"
  }

  depends_on = [
    google_project_service.enabled_apis["secretmanager.googleapis.com"]
  ]
}

resource "google_secret_manager_secret_version" "grafana_admin_password" {
  secret      = google_secret_manager_secret.grafana_admin_password.id
  secret_data = var.grafana_admin_password
}

resource "google_secret_manager_secret_iam_member" "grafana_admin_password_accessor" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.grafana_admin_password.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.ops_copilot.email}"
}

# 2. Optional secret for Gemini Developer API Key
resource "google_secret_manager_secret" "gemini_api_key" {
  secret_id = "gemini-api-key"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = {
    managed-by = "terraform"
    service    = "ai-ops-copilot"
  }

  depends_on = [
    google_project_service.enabled_apis["secretmanager.googleapis.com"]
  ]
}

resource "google_secret_manager_secret_iam_member" "gemini_api_key_accessor" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.gemini_api_key.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.ops_copilot.email}"
}

resource "google_secret_manager_secret_version" "gemini_api_key" {
  count       = var.gemini_api_key != "" ? 1 : 0
  secret      = google_secret_manager_secret.gemini_api_key.id
  secret_data = var.gemini_api_key
}
