locals {
  ubuntu_image_url = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
  cloud_init_data = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    admin_username           = var.admin_username
    ssh_public_key           = var.ssh_public_key
    netbird_setup_key_b64    = base64encode(data.external.secrets.result.netbird_setup_key)
    immich_database_password = base64encode(data.external.secrets.result.immich_database_password)
    immich_version            = var.immich_version
    timezone                  = var.timezone
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
    file_name = "immich-cloud-init.yaml"
  }
}

resource "proxmox_virtual_environment_vm" "immich" {
  name      = "immich"
  vm_id     = 100
  node_name = var.proxmox_node

  on_boot       = true
  started       = true
  stop_on_destroy = true

  cpu {
    cores = 4
    type  = "host"
  }

  memory {
    dedicated = 6144
  }

  agent {
    enabled = true
  }

  disk {
    datastore_id = var.vm_storage
    file_id      = proxmox_download_file.ubuntu_cloud_image.id
    interface    = "scsi0"
    size         = 64
    file_format  = "raw"
    iothread     = true

    discard = "on"

    # Ubuntu cloud images use SCSI reliably with the virtio-scsi controller.
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
