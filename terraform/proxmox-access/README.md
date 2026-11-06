# Proxmox access

Proxmox users, roles, pools and ACLs for the runner controller. It is its own Terraform root so that rebuilding the template or the controller container never plans against permissions.

| Resource | What it is |
|---|---|
| Pool `gha-runners` | Holds runner clones. Pool ACLs apply to every VM in it. The template is deliberately not a member. |
| User `gha-controller@pve` | The controller's identity. It has no password and cannot log in. |
| Roles `GhaRunnerPool`, `GhaTemplateClone`, `GhaRunnerStorage`, `GhaRunnerNetwork` | Each privilege is the one a controller API call requires. |
| ACLs on `/pool/gha-runners`, `/storage/local-lvm`, `/sdn/zones/localnetwork/vmbr0` | Grant those roles to the user. |

Two pieces live elsewhere:

- **The grant on the template** (`VM.Clone` on `/vms/910`) is in `terraform/runner-template/`, because Proxmox deletes it whenever the template VM is destroyed.
- **The API token** is created by hand, so its secret never reaches Terraform state. It uses `privsep 0`, so it has exactly the user's ACLs and needs none of its own.

Which privilege each API call needs, and why each grant is as broad as it is, is in `docs/runner-controller-access.md`.

## Credentials

This root uses the same Terraform token as the other roots (`PROXMOX_VE_API_TOKEN`). That token, `root@pam!terraform`, is privilege-separated, so it also needs grants of its own to manage users, roles, pools and ACLs. Each privilege below is the one the Proxmox API schema requires for the call:

| Role | Privileges | Granted on | Needed for |
|---|---|---|---|
| `TerraformAccess` | `Sys.Audit`, `Sys.Modify`, `User.Modify` | `/access` | Reading and updating the user, creating and updating roles |
| `TerraformAcl` | `Permissions.Modify`, `Pool.Audit`, `Pool.Allocate` | `/pool/gha-runners`, `/storage/local-lvm`, `/sdn/zones/localnetwork/vmbr0`, `/vms` | Managing ACLs on those paths, and the pool |

`TerraformAcl` is on all of `/vms` rather than `/vms/910`, because Proxmox deletes ACLs on a VM when it is destroyed, and the template is rebuilt.

`Permissions.Modify` lets the token grant any role on those paths, itself included, so it can give itself full control of every VM. Guard the token like root's password.

A token cannot grant itself access it does not have yet, so these grants are made once by hand. On pve:

```sh
pveum role add TerraformAccess --privs "Sys.Audit,Sys.Modify,User.Modify"
pveum role add TerraformAcl    --privs "Permissions.Modify,Pool.Audit,Pool.Allocate"
pveum acl modify /access --tokens 'root@pam!terraform' --roles TerraformAccess
for p in /pool/gha-runners /storage/local-lvm /sdn/zones/localnetwork/vmbr0 /vms; do
  pveum acl modify "$p" --tokens 'root@pam!terraform' --roles TerraformAcl
done
```

## Apply

Apply this root before `terraform/runner-template/`, whose template grant refers to the user and role created here.

```sh
cd terraform/proxmox-access
cp terraform.tfvars.example terraform.tfvars   # fill in
terraform init
terraform plan    # new setup: pool, user, 4 roles, 3 ACLs
terraform apply
```

Then create the token. `pveum` prints its secret once; it goes into `PROXMOX_API_TOKEN` in the controller's `controller.env` as `gha-controller@pve!controller=<secret>`. On pve:

```sh
pveum user token add gha-controller@pve controller --privsep 0 --comment "gha runner controller"
```

## Adopting objects made by hand (once)

Setups from before this root existed already have these objects, created with `pveum`. Import them instead of creating them, or the apply fails because they already exist:

```sh
cd terraform/proxmox-access
terraform init
terraform import proxmox_virtual_environment_pool.runners gha-runners
terraform import proxmox_virtual_environment_user.controller gha-controller@pve
terraform import proxmox_virtual_environment_role.runner_pool GhaRunnerPool
terraform import proxmox_virtual_environment_role.template_clone GhaTemplateClone
terraform import proxmox_virtual_environment_role.runner_storage GhaRunnerStorage
terraform import proxmox_virtual_environment_role.runner_network GhaRunnerNetwork
terraform import proxmox_acl.runner_pool '/pool/gha-runners?gha-controller@pve?GhaRunnerPool'
terraform import proxmox_acl.runner_storage '/storage/local-lvm?gha-controller@pve?GhaRunnerStorage'
terraform import proxmox_acl.runner_network '/sdn/zones/localnetwork/vmbr0?gha-controller@pve?GhaRunnerNetwork'
terraform plan    # expect no changes, or at most an in-place comment update

cd ../runner-template
terraform import proxmox_acl.template_clone '/vms/910?gha-controller@pve?GhaTemplateClone'
terraform plan    # expect no changes
```

Stop if either plan wants to destroy or replace anything. Destroying the user also deletes its token.

The existing token keeps working throughout. Terraform never touches it.

## Verify

From the controller container, the token's effective permissions should list only the pool and its VMs, `/vms/910`, `/storage/local-lvm` and `/sdn/zones/localnetwork/vmbr0`:

```sh
curl -s -H "Authorization: PVEAPIToken=gha-controller@pve!controller=<secret>" \
  https://pve:8006/api2/json/access/permissions
```
