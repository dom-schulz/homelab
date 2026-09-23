provider "proxmox" {
  endpoint = var.proxmox_api_url
  insecure = var.proxmox_insecure_tls
  ssh {
    agent    = true
    username = var.proxmox_ssh_username
    node {
      name    = var.proxmox_node
      address = var.proxmox_ssh_address
    }
  }
}
