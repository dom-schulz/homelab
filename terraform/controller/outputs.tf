output "controller_vm_id" {
  value = proxmox_virtual_environment_container.controller.vm_id
}

output "ipv4" {
  description = "Addresses per interface; use eth0's as root@<address> for scripts/deploy-controller.sh. May be empty right after creation: run terraform refresh."
  value       = proxmox_virtual_environment_container.controller.ipv4
}
