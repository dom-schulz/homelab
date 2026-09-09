output "vm_id" {
  description = "Proxmox VM ID."
  value       = proxmox_virtual_environment_vm.immich.vm_id
}

output "vm_name" {
  description = "Proxmox VM name."
  value       = proxmox_virtual_environment_vm.immich.name
}

output "vm_ipv4_addresses" {
  description = "IPv4 addresses reported by the QEMU guest agent after boot."
  value       = proxmox_virtual_environment_vm.immich.ipv4_addresses
}

output "immich_url_hint" {
  description = "Use the VM's NetBird address with this port after enrollment."
  value       = "http://<immich-netbird-ip>:2283"
}
