# Users, roles, pools and ACLs all go through the API; no SSH needed.
provider "proxmox" {
  endpoint = var.proxmox_api_url
  insecure = var.proxmox_insecure_tls
}
