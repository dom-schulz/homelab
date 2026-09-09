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

data "external" "secrets" {
  program = ["bash", "-ceu", <<-EOT
    netbird_setup_key="$${NETBIRD_SETUP_KEY:-}"
    immich_database_password="$${IMMICH_DATABASE_PASSWORD:-}"
    test -n "$netbird_setup_key" || { echo "NETBIRD_SETUP_KEY is required" >&2; exit 1; }
    test -n "$immich_database_password" || { echo "IMMICH_DATABASE_PASSWORD is required" >&2; exit 1; }
    command -v jq >/dev/null || { echo "jq is required on the Terraform workstation" >&2; exit 1; }
    jq -n --arg netbird "$netbird_setup_key" --arg database "$immich_database_password" \
      '{netbird_setup_key: $netbird, immich_database_password: $database}'
  EOT
  ]
}
