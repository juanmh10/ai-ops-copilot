# Cloud Run v2 Multi-Container Service (Grafana Ingress + Sidecar Agent ADK)
resource "google_cloud_run_v2_service" "copilot" {
  provider = google-beta

  name     = var.service_name
  location = var.region
  project  = var.project_id

  deletion_protection = false

  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    execution_environment = "EXECUTION_ENVIRONMENT_GEN2"
    service_account       = google_service_account.ops_copilot.email

    scaling {
      min_instance_count = 0
      max_instance_count = 1
    }

    # Container 1: Ingress Gateway & SRE Agent (Receives HTTP traffic on port 8080 and reverse proxies)
    containers {
      name  = "agent-backend"
      image = var.agent_backend_image

      ports {
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = "1000m"
          memory = "512Mi"
        }
      }

      env {
        name  = "GCP_PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "VERTEX_PROJECT_ID"
        value = var.vertex_project_id
      }
      env {
        name  = "FIRESTORE_DATABASE"
        value = "(default)"
      }
      env {
        name  = "GEMINI_MODEL"
        value = "gemini-3.8-flash"
      }
      env {
        name  = "BILLING_PROJECT_ID"
        value = var.billing_project_id
      }
      env {
        name  = "BILLING_DATASET"
        value = trimprefix(var.billing_dataset, "${var.billing_project_id}.")
      }
      env {
        name  = "GRAFANA_LOCAL_URL"
        value = "http://localhost:3000"
      }
    }

    # Container 2: Grafana UI (Visualization server on internal port 3000)
    containers {
      name  = "grafana"
      image = var.grafana_image

      resources {
        limits = {
          cpu    = "1000m"
          memory = "1024Mi"
        }
      }

      env {
        name  = "GF_SERVER_HTTP_PORT"
        value = "3000"
      }
      env {
        name  = "GF_SECURITY_ADMIN_USER"
        value = var.grafana_admin_user
      }
      env {
        name = "GF_SECURITY_ADMIN_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.grafana_admin_password.secret_id
            version = "latest"
          }
        }
      }
      env {
        name  = "GF_AUTH_ANONYMOUS_ENABLED"
        value = "false"
      }
      env {
        name  = "GF_USERS_ALLOW_SIGN_UP"
        value = "false"
      }
      env {
        name  = "GCP_PROJECT_ID"
        value = var.monitoring_project_id
      }
      env {
        name  = "BILLING_PROJECT_ID"
        value = var.billing_project_id
      }
      env {
        name  = "BILLING_DATASET"
        value = var.billing_dataset
      }
      env {
        name  = "GF_SERVER_ROOT_URL"
        value = var.grafana_root_url
      }
      env {
        name  = "GF_LIVE_ALLOWED_ORIGINS"
        value = "*"
      }
      env {
        name  = "GF_SECURITY_CSRF_TRUSTED_ORIGINS"
        value = var.grafana_csrf_trusted_origins
      }
      # Allow loading unsigned AI-Ops Copilot plugin
      env {
        name  = "GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS"
        value = "ai-ops-copilot-chat"
      }
      # Internal sidecar URL for chat plugin queries
      env {
        name  = "AGENT_BACKEND_URL"
        value = "http://localhost:8080"
      }
      env {
        name  = "GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH"
        value = "/etc/grafana/provisioning/dashboards/json/unified-cloudrun-dashboard.json"
      }
    }
  }

  labels = {
    managed-by = "terraform"
    service    = "ai-ops-copilot"
    stack      = "serverless-observability"
  }

  # The CD pipeline deploys only new images. Terraform maintains
  # scaling, resources, volumes, identity, and environment variables.
  lifecycle {
    ignore_changes = [
      template[0].containers[0].image,
      template[0].containers[1].image,
    ]
  }

  depends_on = [
    google_project_service.enabled_apis["run.googleapis.com"],
    google_firestore_database.database,
    google_storage_bucket_iam_member.grafana_storage_admin,
    google_project_iam_member.firestore_user,
    google_project_iam_member.monitoring_viewer,
    google_bigquery_dataset_iam_member.billing_data_viewer,
    google_project_iam_member.billing_job_user,
    google_project_iam_member.billing_service_usage_consumer,
    google_project_iam_member.vertex_ai_user,
    google_secret_manager_secret_iam_member.grafana_admin_password_accessor,
    google_secret_manager_secret_version.grafana_admin_password,
  ]
}
