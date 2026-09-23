output "builder_vm_id" {
  description = "Temporary VM ID used to build the runner template."
  value = proxmox_virtual_environment_vm.runner_template_builder.vm_id
}
output "builder_vm_name" {
  description = "Temporary builder VM name."
  value = proxmox_virtual_environment_vm.runner_template_builder.name
}
output "template_conversion_note" {
  description = "Cleanup and conversion to a Proxmox template are deliberate operator actions."
  value = "Run /usr/local/sbin/prepare-runner-template, shut down VM ${var.builder_vm_id}, then convert it to a Proxmox template."
}
