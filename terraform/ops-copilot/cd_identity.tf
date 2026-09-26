# Dedicated Workload Identity Federation for continuous deployment of AI-Ops Copilot.
# The provider accepts only OIDC tokens from this repository on the main branch.
data "google_project" "host" {
  project_id = var.project_id
}

resource "google_iam_workload_identity_pool" "github_cd" {
  project                   = var.project_id
  workload_identity_pool_id = "github-ops-copilot"
  display_name              = "GitHub AI-Ops Copilot CD"
  description               = "Federated identity for publishing AI-Ops Copilot container images"

  depends_on = [google_project_service.enabled_apis["iam.googleapis.com"]]
}

resource "google_iam_workload_identity_pool_provider" "github_main" {
  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github_cd.workload_identity_pool_id
  workload_identity_pool_provider_id = "infra-main"
  display_name                       = "infra main"

  attribute_mapping = {
    "google.subject"                = "assertion.sub"
    "attribute.repository_id"       = "assertion.repository_id"
    "attribute.repository_owner_id" = "assertion.repository_owner_id"
    "attribute.ref"                 = "assertion.ref"
    "attribute.workflow_ref"        = "assertion.workflow_ref"
  }
  attribute_condition = "assertion.repository_id == '${var.github_repository_id}' && assertion.repository_owner_id == '${var.github_repository_owner_id}' && assertion.ref == 'refs/heads/main' && attribute.workflow_ref.endsWith('/.github/workflows/ci.yml@refs/heads/main')"

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }

  depends_on = [google_project_service.enabled_apis["sts.googleapis.com"]]
}

resource "google_service_account" "github_deployer" {
  project      = var.project_id
  account_id   = "ops-copilot-deployer"
  display_name = "AI-Ops Copilot GitHub Deployer"
  description  = "Publishes images and updates only the Cloud Run Copilot service"

  depends_on = [google_project_service.enabled_apis["iam.googleapis.com"]]
}

resource "google_service_account_iam_member" "github_wif" {
  service_account_id = google_service_account.github_deployer.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/projects/${data.google_project.host.number}/locations/global/workloadIdentityPools/${google_iam_workload_identity_pool.github_cd.workload_identity_pool_id}/attribute.repository_id/${var.github_repository_id}"
}

resource "google_artifact_registry_repository_iam_member" "github_artifact_writer" {
  project    = var.project_id
  location   = var.region
  repository = google_artifact_registry_repository.copilot_repo.repository_id
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.github_deployer.email}"
}

resource "google_cloud_run_v2_service_iam_member" "github_run_developer" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.copilot.name
  role     = "roles/run.developer"
  member   = "serviceAccount:${google_service_account.github_deployer.email}"
}

resource "google_service_account_iam_member" "github_runtime_act_as" {
  service_account_id = google_service_account.ops_copilot.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.github_deployer.email}"
}
