# https://registry.terraform.io/providers/hashicorp/google/latest/docs
provider "google" {
  project = "family-meeting-aa853"
  region  = "us-central1"
}

# https://www.terraform.io/language/settings/backends/gcs
terraform {
  backend "gcs" {
    bucket = "family-meeting-generic"
    prefix = "terraform/state"
  }
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 4.0"
    }
  }
}
