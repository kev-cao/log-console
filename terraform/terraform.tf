locals {
  environment = terraform.workspace
}

module "vars" {
  source      = "./modules/vars"
  environment = local.environment
}

terraform {
  backend "gcs" {
    bucket = "log-console-terraform-state"
  }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "4.51.0"
    }
  }
}

provider "google" {
  project = module.vars.env.project_id
}

