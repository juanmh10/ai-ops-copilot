# Cloud Firestore database in Native mode
resource "google_firestore_database" "database" {
  project     = var.project_id
  name        = "(default)"
  location_id = var.region
  type        = "FIRESTORE_NATIVE"

  # Configuration to allow clean terraform destroy in test/dev lifecycle
  delete_protection_state = "DELETE_PROTECTION_DISABLED"
  deletion_policy         = "DELETE"

  depends_on = [
    google_project_service.enabled_apis["firestore.googleapis.com"]
  ]
}
