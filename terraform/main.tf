locals {
  ubuntu_image_url = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
  cloud_init_data = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    admin_username = var.admin_username
    ssh_public_key = var.ssh_public_key
    hostname       = var.builder_name
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
  stop_on_destroy = true
  cpu {
    cores = var.builder_cpu_cores
    type  = "host"
  }
  memory {
    dedicated = var.builder_memory_mb
  }
  agent {
    enabled = true
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
    bridge = var.network_bridge
    model  = "virtio"
  }
}
