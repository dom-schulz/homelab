locals {
  ubuntu_image_url = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
  cloud_init_data = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    admin_username         = var.admin_username
    builder_debug_password = var.builder_debug_password
    ssh_public_key         = var.ssh_public_key
    hostname               = var.builder_name
    runner_version         = var.runner_version
    runner_sha256          = var.runner_sha256
  })
}
resource "proxmox_download_file" "ubuntu_cloud_image" {
  content_type = "iso"
  datastore_id = var.image_storage
  node_name    = var.proxmox_node
  file_name    = "noble-server-cloudimg-amd64.img"
  url          = local.ubuntu_image_url
}
resource "proxmox_virtual_environment_file" "cloud_init" {
  content_type = "snippets"
  datastore_id = var.snippet_storage
  node_name    = var.proxmox_node
  source_raw {
    data      = local.cloud_init_data
    file_name = "${var.builder_name}-cloud-init.yaml"
  }
}
resource "proxmox_virtual_environment_vm" "runner_template_builder" {
  name            = var.builder_name
  vm_id           = var.builder_vm_id
  node_name       = var.proxmox_node
  description     = "Temporary builder for the ephemeral GitHub Actions runner template; convert manually after validation."
  on_boot         = false
  started         = false
  template        = var.builder_is_template
  stop_on_destroy = true
  cpu {
    cores = var.builder_cpu_cores
    type  = "host"
  }
  memory {
    dedicated = var.builder_memory_mb
  }
  agent {
    # Keep the virtio agent channel, but do not wait for an IP: this builder is
    # created stopped and cloud-init installs the agent after a manual start.
    enabled = true
    wait_for_ip {
      disabled = true
    }
  }
  disk {
    datastore_id = var.vm_storage
    file_id      = proxmox_download_file.ubuntu_cloud_image.id
    interface    = "scsi0"
    size         = var.builder_disk_gb
    file_format  = "raw"
    iothread     = true
    discard      = "on"
  }
  scsi_hardware = "virtio-scsi-single"
  boot_order    = ["scsi0"]
  initialization {
    datastore_id      = var.vm_storage
    user_data_file_id = proxmox_virtual_environment_file.cloud_init.id
    ip_config {
      ipv4 {
        address = var.vm_ipv4_address
        gateway = var.vm_ipv4_gateway
      }
    }
  }
  network_device {
    bridge   = var.network_bridge
    model    = "virtio"
    firewall = true
  }
}

# Firewall for the template. qm clone copies the template's firewall config, so
# every runner VM gets these rules. They only take effect once the datacenter
# firewall is enabled; see docs/runner-isolation.md.
resource "proxmox_virtual_environment_firewall_options" "runner_template" {
  node_name     = var.proxmox_node
  vm_id         = proxmox_virtual_environment_vm.runner_template_builder.vm_id
  enabled       = true
  dhcp          = true
  ndp           = false # IPv6 is rejected anyway
  radv          = false # never let a runner advertise itself as an IPv6 router
  macfilter     = true
  input_policy  = "DROP"
  output_policy = "ACCEPT"

  # Rebuilding VM 910 deletes its firewall config; recreate it with the VM.
  lifecycle {
    replace_triggered_by = [proxmox_virtual_environment_vm.runner_template_builder]
  }
}

# Rules match in order: allow DNS to the resolver, drop every private, NetBird
# and link-local IPv4 destination, and reject all IPv6 (LAN hosts have global
# IPv6 addresses; rejecting makes clients fall back to IPv4 at once). The rest of
# IPv4 (the internet) is allowed by the outbound policy.
resource "proxmox_virtual_environment_firewall_rules" "runner_template" {
  node_name = var.proxmox_node
  vm_id     = proxmox_virtual_environment_vm.runner_template_builder.vm_id

  dynamic "rule" {
    for_each = setproduct(var.runner_dns_servers, ["udp", "tcp"])
    content {
      type    = "out"
      action  = "ACCEPT"
      dest    = rule.value[0]
      proto   = rule.value[1]
      dport   = "53"
      comment = "DNS"
    }
  }
  dynamic "rule" {
    for_each = var.runner_blocked_ipv4
    content {
      type    = "out"
      action  = "DROP"
      dest    = rule.value
      comment = "No local network access"
    }
  }
  rule {
    type    = "out"
    action  = "REJECT"
    dest    = "::/0"
    comment = "No IPv6"
  }

  depends_on = [proxmox_virtual_environment_firewall_options.runner_template]

  lifecycle {
    replace_triggered_by = [proxmox_virtual_environment_vm.runner_template_builder]
  }
}
