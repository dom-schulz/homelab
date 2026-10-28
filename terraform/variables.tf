variable "proxmox_api_url" {
  type = string
}

variable "proxmox_insecure_tls" {
  type    = bool
  default = false
}

variable "proxmox_node" {
  type = string
}

variable "proxmox_ssh_address" {
  type = string
}

variable "proxmox_ssh_username" {
  type    = string
  default = "root"
}

variable "vm_storage" {
  type = string
}

variable "image_storage" {
  type = string
}

variable "snippet_storage" {
  type = string
}

variable "network_bridge" {
  type    = string
  default = "vmbr0"
}

variable "vm_ipv4_address" {
  type    = string
  default = "dhcp"
}

variable "vm_ipv4_gateway" {
  type     = string
  default  = null
  nullable = true
}

variable "admin_username" {
  type    = string
  default = "ubuntu"
}

variable "runner_version" {
  type        = string
  default     = "2.338.1"
  description = "actions/runner release baked into the template; runners auto-update at startup when it falls behind."
}

variable "runner_sha256" {
  type        = string
  default     = "08e9ccb3604d2462c24b5e33604dc30810296ca15cc65f23cffd4317eb144c37"
  description = "SHA256 of actions-runner-linux-x64-<runner_version>.tar.gz from the release notes."
}

variable "builder_debug_password" {
  type        = string
  sensitive   = true
  nullable    = true
  default     = null
  description = "Temporary password for console debugging; omit before converting the VM to a template."

  validation {
    condition     = var.builder_debug_password == null || length(var.builder_debug_password) >= 12
    error_message = "builder_debug_password must be at least 12 characters when provided."
  }
}

variable "ssh_public_key" {
  type = string
}

variable "builder_name" {
  type    = string
  default = "gha-runner-template-builder"
}

variable "builder_is_template" {
  type        = bool
  default     = false
  description = "Set to true after the builder is cleaned and converted with qm template; Proxmox cannot convert a template back to a VM."
}

variable "builder_vm_id" {
  type = number
}

variable "builder_cpu_cores" {
  type    = number
  default = 2
}

variable "builder_memory_mb" {
  type    = number
  default = 2048
}

variable "builder_disk_gb" {
  type    = number
  default = 16
}
