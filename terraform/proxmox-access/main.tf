# Proxmox access for the runner controller. Each privilege is the one its API
# call requires; see docs/runner-controller-access.md. The grant on the template
# (VM.Clone on /vms/910) lives in terraform/runner-template, so it is recreated
# with the template. The API token is created by hand so its secret never
# reaches Terraform state.

resource "proxmox_virtual_environment_pool" "runners" {
  pool_id = var.runner_pool
  comment = "Ephemeral GitHub Actions runner clones"
}

resource "proxmox_virtual_environment_user" "controller" {
  user_id = var.controller_user
  comment = "Runner controller, no login"
}

resource "proxmox_virtual_environment_role" "runner_pool" {
  role_id = "GhaRunnerPool"
  privileges = [
    "Pool.Audit",
    "VM.Allocate",
    "VM.Audit",
    "VM.Config.Cloudinit",
    "VM.GuestAgent.Audit",
    "VM.GuestAgent.FileRead",
    "VM.GuestAgent.FileWrite",
    "VM.PowerMgmt",
  ]
}

resource "proxmox_virtual_environment_role" "template_clone" {
  role_id    = "GhaTemplateClone"
  privileges = ["VM.Clone"]
}

resource "proxmox_virtual_environment_role" "runner_storage" {
  role_id    = "GhaRunnerStorage"
  privileges = ["Datastore.AllocateSpace"]
}

resource "proxmox_virtual_environment_role" "runner_network" {
  role_id    = "GhaRunnerNetwork"
  privileges = ["SDN.Use"]
}

# Pool ACLs apply to every VM in the pool.
resource "proxmox_acl" "runner_pool" {
  path      = "/pool/${proxmox_virtual_environment_pool.runners.pool_id}"
  user_id   = proxmox_virtual_environment_user.controller.user_id
  role_id   = proxmox_virtual_environment_role.runner_pool.role_id
  propagate = true
}

resource "proxmox_acl" "runner_storage" {
  path      = "/storage/${var.vm_storage}"
  user_id   = proxmox_virtual_environment_user.controller.user_id
  role_id   = proxmox_virtual_environment_role.runner_storage.role_id
  propagate = true
}

# A plain Linux bridge is checked as part of the built-in localnetwork zone.
resource "proxmox_acl" "runner_network" {
  path      = "/sdn/zones/localnetwork/${var.network_bridge}"
  user_id   = proxmox_virtual_environment_user.controller.user_id
  role_id   = proxmox_virtual_environment_role.runner_network.role_id
  propagate = true
}
