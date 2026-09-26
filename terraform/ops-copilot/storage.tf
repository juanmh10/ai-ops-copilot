# GCS Bucket for Grafana persistent data (/var/lib/grafana)
# Mounted via Cloud Run GCS FUSE volume mount
resource "google_storage_bucket" "grafana_data" {
  name                        = "${var.project_id}-ops-grafana-data"
  location                    = var.region
  project                     = var.project_id
  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  force_destroy               = true

  versioning {
    enabled = true
  }

  lifecycle_rule {
    action {
      type = "Delete"
    }
    condition {
      num_newer_versions = 3
      with_state         = "ARCHIVED"
    }
  }

  labels = {
    managed-by = "terraform"
    service    = "ai-ops-copilot"
    component  = "grafana-storage"
  }
}
