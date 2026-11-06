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

variable "template_storage" {
  description = "Storage that holds LXC templates (content type vztmpl)."
  type        = string
  default     = "local"
}

variable "container_storage" {
  type    = string
  default = "local-lvm"
}

variable "network_bridge" {
  type    = string
  default = "vmbr0"
}

variable "controller_vm_id" {
  type = number
}

variable "controller_hostname" {
  type    = string
  default = "gha-controller"
}

variable "dns_servers" {
  description = "Resolvers the controller may reach on port 53, usually the LAN router."
  type        = list(string)
  default     = ["10.0.0.1"]
}

variable "api_bridge" {
  description = "Bridge with no physical port that joins pve and the controller only."
  type        = string
  default     = "vmbr1"
}

variable "api_network_address" {
  description = "The controller's static address on api_bridge, in CIDR notation."
  type        = string
  default     = "10.10.10.2/24"
}

variable "proxmox_api_address" {
  description = "pve's address on api_bridge (port 8006), the only local destination the controller may reach."
  type        = string
  default     = "10.10.10.1"

  validation {
    condition     = can(cidrhost("${var.proxmox_api_address}/32", 0))
    error_message = "proxmox_api_address must be a plain IPv4 address (pve's address on api_bridge)."
  }
}

variable "ssh_sources" {
  description = "Sources allowed to SSH in. NetBird traffic arrives masqueraded as the Proxmox host's LAN address."
  type        = list(string)
  default     = ["10.0.0.0/24"]
}

variable "blocked_ipv4" {
  description = "Other local destinations the controller may not reach: private, CGNAT/NetBird and link-local ranges."
  type        = list(string)
  default     = ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "169.254.0.0/16"]
}

variable "ssh_public_key" {
  description = "Authorized for root; used by scripts/deploy-controller.sh."
  type        = string
}

variable "cpu_cores" {
  type    = number
  default = 1
}

variable "memory_mb" {
  type    = number
  default = 512
}

variable "disk_gb" {
  type    = number
  default = 4
}
