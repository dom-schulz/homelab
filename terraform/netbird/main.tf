data "netbird_peer" "admin_workstation" {
  name = "dschulz-HP-Pavilion"
}

data "netbird_peer" "routing_peer" {
  name = "proxmox-host"
}

resource "netbird_group" "admin_workstation" {
  name = "homelab-admin-workstation"
  peers = [data.netbird_peer.admin_workstation.id]
}
resource "netbird_group" "routing_peer" {
  name = "homelab-routing-peer"
  peers = [data.netbird_peer.routing_peer.id]
}
resource "netbird_group" "runner_resource" {
  name = "homelab-runner-resource"
}
resource "netbird_network" "homelab" {
  name = "homelab"
  description = "Resources routed through the Proxmox NetBird peer."
}
resource "netbird_network_resource" "runner" {
  network_id = netbird_network.homelab.id
  name = "gha-runner-template-builder"
  description = "Ephemeral GitHub Actions runner template builder VM."
  address = var.runner_address
  groups = [netbird_group.runner_resource.id]
  enabled = true
}
resource "netbird_network_router" "proxmox" {
  network_id = netbird_network.homelab.id
  peer_groups = [netbird_group.routing_peer.id]
  enabled = true
  masquerade = true
}
resource "netbird_policy" "proxmox_admin" {
  name = "homelab-admin-to-proxmox"
  description = "Allow the HP workstation to administer Proxmox over SSH."
  enabled = true
  rule {
    name = "ssh"
    description = "Proxmox SSH administration"
    enabled = true
    action = "accept"
    bidirectional = true
    protocol = "tcp"
    ports = ["22"]
    sources = [netbird_group.admin_workstation.id]
    destinations = [netbird_group.routing_peer.id]
  }
}

resource "netbird_policy" "proxmox_web_ui" {
  name = "homelab-admin-to-proxmox-web-ui"
  description = "Allow the HP workstation to reach the Proxmox web UI and API."
  enabled = true
  rule {
    name = "web-ui"
    description = "Proxmox web UI and API"
    enabled = true
    action = "accept"
    bidirectional = true
    protocol = "tcp"
    ports = ["8006"]
    sources = [netbird_group.admin_workstation.id]
    destinations = [netbird_group.routing_peer.id]
  }
}
resource "netbird_policy" "runner_ssh" {
  name = "homelab-admin-to-runner"
  description = "Allow the HP workstation to reach the runner VM over SSH."
  enabled = true
  rule {
    name = "ssh"
    description = "Runner SSH administration"
    enabled = true
    action = "accept"
    bidirectional = true
    protocol = "tcp"
    ports = ["22"]
    sources = [netbird_group.admin_workstation.id]
    destinations = [netbird_group.runner_resource.id]
  }
}
