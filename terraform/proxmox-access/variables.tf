variable "proxmox_api_url" {
  type = string
}

variable "proxmox_insecure_tls" {
  type    = bool
  default = false
}

variable "controller_user" {
  description = "Proxmox user the controller's API token belongs to."
  type        = string
  default     = "gha-controller@pve"
}

variable "runner_pool" {
  description = "Pool that holds runner clones. The template must not be in it."
  type        = string
  default     = "gha-runners"
}

variable "vm_storage" {
  description = "Storage that holds the template and its linked clones."
  type        = string
  default     = "local-lvm"
}

variable "network_bridge" {
  type    = string
  default = "vmbr0"
}
