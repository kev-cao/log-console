locals {
  environment = terraform.workspace
}

module "vars" {
  source      = "../vars"
  environment = local.environment
}

provider "google" {
  project = module.vars.env.project_id
}

resource "google_storage_bucket" "terraform_state_bucket" {
  name          = "log-console-terraform-state"
  location      = var.gcloud_region
  force_destroy = true
  storage_class = "STANDARD"

  versioning {
    enabled = true
  }

  uniform_bucket_level_access = true
}
