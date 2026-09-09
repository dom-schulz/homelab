variable "proxmox_api_url" {
  description = "Proxmox API endpoint, for example https://pve.example:8006/"
  type        = string
}

variable "proxmox_insecure_tls" {
  description = "Disable TLS certificate verification for a private/self-signed Proxmox endpoint."
  type        = bool
  default     = false
}

variable "proxmox_node" {
  description = "Proxmox node on which to create the VM."
  type        = string
}

variable "proxmox_ssh_address" {
  description = "IP address or DNS name used for SSH uploads to the Proxmox node."
  type        = string
}

variable "proxmox_ssh_username" {
  description = "SSH user for uploading the cloud-init snippet; root is simplest for this provider operation."
  type        = string
  default     = "root"
}

variable "vm_storage" {
  description = "Storage for the VM disk and cloud-init drive."
  type        = string
}

variable "image_storage" {
  description = "File storage for the downloaded Ubuntu cloud image."
  type        = string
}

variable "snippet_storage" {
  description = "File storage with the Snippets content type enabled."
  type        = string
}

variable "network_bridge" {
  description = "Proxmox network bridge."
  type        = string
  default     = "vmbr0"
}

variable "vm_ipv4_address" {
  description = "IPv4 address in CIDR notation, or dhcp."
  type        = string
  default     = "dhcp"
}

variable "vm_ipv4_gateway" {
  description = "IPv4 gateway for a static address. Leave null when using DHCP."
  type        = string
  default     = null
  nullable    = true
}

variable "admin_username" {
  description = "Administrative Ubuntu username created by cloud-init."
  type        = string
  default     = "ubuntu"
}

variable "ssh_public_key" {
  description = "SSH public key installed for the administrative user."
  type        = string
}

variable "immich_version" {
  description = "Pinned Immich release tag, including the leading v (for example v1.132.3)."
  type        = string
}

variable "timezone" {
  description = "Timezone used by Immich."
  type        = string
  default     = "America/Los_Angeles"
}
