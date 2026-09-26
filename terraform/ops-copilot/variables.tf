variable "project_id" {
  description = "GCP host project ID for Grafana and Copilot infrastructure"
  type        = string
  default     = "enterprise-ops-platform"
}

variable "monitoring_project_id" {
  description = "Central Cloud Monitoring scoping project ID for Grafana"
  type        = string
  default     = "enterprise-core-prod"
}

variable "region" {
  description = "Default Google Cloud region for Cloud Run and Storage"
  type        = string
  default     = "us-central1"
}

variable "service_name" {
  description = "Name of the Cloud Run multi-container service"
  type        = string
  default     = "ai-ops-copilot"
}

variable "grafana_image" {
  description = "Initial container image for Grafana; subsequent revisions are deployed via CD"
  type        = string
}

variable "agent_backend_image" {
  description = "Initial container image for the Go backend; subsequent revisions are deployed via CD"
  type        = string
}

variable "github_repository_id" {
  description = "Numeric GitHub repository ID authorized for WIF OIDC publication"
  type        = string
}

variable "github_repository_owner_id" {
  description = "Numeric GitHub account/owner ID authorized for WIF OIDC publication"
  type        = string
}

variable "grafana_root_url" {
  description = "Public root URL for Grafana (e.g., Cloud Run service URL or custom domain)"
  type        = string
  default     = "https://ai-ops-copilot.example.com"
}

variable "grafana_csrf_trusted_origins" {
  description = "Comma-separated list of trusted origins for Grafana CSRF protection"
  type        = string
  default     = "ai-ops-copilot.example.com"
}

variable "authorized_user_email" {
  description = "Authorized operator email for Cloud Run invoker IAM role"
  type        = string
  default     = "ops-lead@example.com"
}

variable "monitored_projects" {
  description = "List of GCP project IDs monitored by the copilot agent and Grafana"
  type        = list(string)
  default = [
    "enterprise-core-prod",
    "enterprise-saas-staging",
    "enterprise-ai-analytics",
    "enterprise-telemetry-prod",
    "enterprise-ai-gateway"
  ]
}

variable "grafana_admin_user" {
  description = "Initial administrator username for Grafana"
  type        = string
  default     = "admin"
}

variable "grafana_admin_password" {
  description = "Initial administrator password for Grafana (injected via secret or TF_VAR)"
  type        = string
  sensitive   = true
}

variable "vertex_project_id" {
  description = "GCP project ID dedicated for Vertex AI / Gemini API quotas and calls"
  type        = string
  default     = "enterprise-ai-gateway"
}

variable "billing_project_id" {
  description = "Project hosting the standard billing export queried by Grafana"
  type        = string
  default     = "enterprise-ai-analytics"
}

variable "billing_dataset" {
  description = "Fully qualified BigQuery dataset for the Grafana billing datasource"
  type        = string
  default     = "enterprise-ai-analytics.billing_export"
}

variable "gemini_api_key" {
  description = "Optional API key for Gemini Developer API fallback (when not using Vertex AI ADC)"
  type        = string
  sensitive   = true
  default     = ""
}
