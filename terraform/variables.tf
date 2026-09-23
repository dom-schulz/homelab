variable "proxmox_api_url" { type = string }
variable "proxmox_insecure_tls" { type = bool default = false }
variable "proxmox_node" { type = string }
variable "proxmox_ssh_address" { type = string }
variable "proxmox_ssh_username" { type = string default = "root" }
variable "vm_storage" { type = string }
variable "image_storage" { type = string }
variable "snippet_storage" { type = string }
variable "network_bridge" { type = string default = "vmbr0" }
variable "vm_ipv4_address" { type = string default = "dhcp" }
variable "vm_ipv4_gateway" { type = string default = null nullable = true }
variable "admin_username" { type = string default = "ubuntu" }
variable "ssh_public_key" { type = string }
variable "builder_name" { type = string default = "gha-runner-template-builder" }
variable "builder_vm_id" { type = number }
variable "builder_cpu_cores" { type = number default = 2 }
variable "builder_memory_mb" { type = number default = 2048 }
variable "builder_disk_gb" { type = number default = 16 }
