terraform {
  required_version = ">= 1.6.0"
  required_providers {
    netbird = {
      source  = "netbirdio/netbird"
      version = "= 0.0.10"
    }
  }
}
provider "netbird" {
  # Reads NB_PAT from the environment; never store the token in Terraform.
}
