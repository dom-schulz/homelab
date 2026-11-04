# Proxmox's Ubuntu 24.04 LXC template. The checksum is the sha512sum from
# http://download.proxmox.com/images/aplinfo-pve-9.dat (what pveam verifies).
resource "proxmox_download_file" "ubuntu_lxc_template" {
  content_type       = "vztmpl"
  datastore_id       = var.template_storage
  node_name          = var.proxmox_node
  url                = "http://download.proxmox.com/images/system/ubuntu-24.04-standard_24.04-2_amd64.tar.zst"
  checksum           = "45c2978e6b97fe292ada95fe06834276015e5739a594db4de2fdfd830fa0c37942e8ae118fc1e32ffd9154b3f9378b592738b668ea3957db41f2907b86f219de"
  checksum_algorithm = "sha512"
}

# Long-lived host for the runner controller. Holds the GitHub App key and the
# Proxmox token, so it is trusted, unlike the runner VMs it creates. Software
# and secrets are installed afterwards; see README.md.
resource "proxmox_virtual_environment_container" "controller" {
  node_name     = var.proxmox_node
  vm_id         = var.controller_vm_id
  description   = "Ephemeral GitHub Actions runner controller."
  tags          = ["gha"]
  unprivileged  = true
  started       = true
  start_on_boot = true

  # systemd uses nesting to sandbox services (PrivateTmp, ProtectSystem).
  features {
    nesting = true
  }

  cpu {
    cores = var.cpu_cores
  }
  memory {
    dedicated = var.memory_mb
    swap      = 0
  }
  disk {
    datastore_id = var.container_storage
    size         = var.disk_gb
  }

  operating_system {
    template_file_id = proxmox_download_file.ubuntu_lxc_template.id
    type             = "ubuntu"
  }

  initialization {
    hostname = var.controller_hostname
    dns {
      servers = var.dns_servers
    }
    ip_config {
      ipv4 {
        address = "dhcp" # only SSH needs the address; the controller connects out
      }
    }
    user_account {
      keys = [var.ssh_public_key]
    }
  }

  network_interface {
    name     = "eth0"
    bridge   = var.network_bridge
    firewall = true
  }
}

resource "proxmox_virtual_environment_firewall_options" "controller" {
  node_name     = var.proxmox_node
  container_id  = proxmox_virtual_environment_container.controller.vm_id
  enabled       = true
  dhcp          = true
  ndp           = false # IPv6 is rejected anyway
  radv          = false
  macfilter     = true
  input_policy  = "DROP"
  output_policy = "ACCEPT"
}

# Rules match in order. In: SSH from the admin sources only. Out: DNS and the
# Proxmox API are the only local destinations; every other private, NetBird and
# link-local IPv4 address is dropped and all IPv6 rejected, as for the runners.
# The rest of IPv4 (GitHub, Ubuntu mirrors) is allowed by the outbound policy.
resource "proxmox_virtual_environment_firewall_rules" "controller" {
  node_name    = var.proxmox_node
  container_id = proxmox_virtual_environment_container.controller.vm_id

  dynamic "rule" {
    for_each = var.ssh_sources
    content {
      type    = "in"
      action  = "ACCEPT"
      source  = rule.value
      proto   = "tcp"
      dport   = "22"
      comment = "SSH"
    }
  }
  dynamic "rule" {
    for_each = setproduct(var.dns_servers, ["udp", "tcp"])
    content {
      type    = "out"
      action  = "ACCEPT"
      dest    = rule.value[0]
      proto   = rule.value[1]
      dport   = "53"
      comment = "DNS"
    }
  }
  rule {
    type    = "out"
    action  = "ACCEPT"
    dest    = var.proxmox_api_address
    proto   = "tcp"
    dport   = "8006"
    comment = "Proxmox API"
  }
  dynamic "rule" {
    for_each = var.blocked_ipv4
    content {
      type    = "out"
      action  = "DROP"
      dest    = rule.value
      comment = "No other local network access"
    }
  }
  rule {
    type    = "out"
    action  = "REJECT"
    dest    = "::/0"
    comment = "No IPv6"
  }

  depends_on = [proxmox_virtual_environment_firewall_options.controller]
}
