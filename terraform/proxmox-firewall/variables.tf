variable "proxmox_api_url" {
  type = string
}

variable "proxmox_insecure_tls" {
  type    = bool
  default = false
}

variable "proxmox_node" {
  type    = string
  default = "pve"
}

variable "admin_netbird_ip" {
  description = "The admin workstation's NetBird address (netbird status). NetBird keeps it per peer."
  type        = string
}

variable "netbird_interface" {
  description = "pve's NetBird interface."
  type        = string
  default     = "wt0"
}

variable "netbird_wireguard_port" {
  description = "UDP port of pve's NetBird agent (netbird status on pve)."
  type        = number
  default     = 51820
}

variable "controller_bridge" {
  description = "Bridge with no physical port that joins pve and the controller only."
  type        = string
  default     = "vmbr1"
}

variable "controller_ip" {
  description = "The controller's static address on controller_bridge."
  type        = string
  default     = "10.10.10.2"
}
