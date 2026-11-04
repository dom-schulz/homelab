output "controller_user" {
  value = proxmox_virtual_environment_user.controller.user_id
}

output "runner_pool" {
  value = proxmox_virtual_environment_pool.runners.pool_id
}
