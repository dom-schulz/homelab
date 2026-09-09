# Immich Terraform variables

`network_bridge` is a virtual Layer-2 switch on the Proxmox host. The VM's virtual network card plugs into it, and the bridge connects the VM to the physical LAN through the host's network interface. `vmbr0` is the usual Proxmox default.

It is not NetBird: NetBird runs inside the VM later and creates a separate encrypted overlay interface. The bridge provides the VM's initial ordinary network access so cloud-init can install Docker, NetBird, and Immich.

## TLS verification

`proxmox_insecure_tls = false` means Terraform verifies that the Proxmox HTTPS certificate is trusted and belongs to the expected host. This protects the API token from being sent to an impersonating endpoint.

Set it to `true` only when the Proxmox host uses a self-signed certificate and the connection is otherwise trusted. A better long-term solution is to give Proxmox a certificate signed by a trusted internal or public CA, then keep this value `false`.

## IPv4 address and gateway

An IPv4 address identifies the VM on the local network, for example `192.168.1.50/24`. The `/24` is the network prefix length.

The IPv4 gateway is the router address, commonly `192.168.1.1`. The VM sends traffic for destinations outside its local subnet—such as the Proxmox API, package repositories, GitHub, or NetBird—through this gateway.

This configuration uses `vm_ipv4_address = "dhcp"` by default. DHCP supplies the VM address, subnet, gateway, and DNS automatically, so `vm_ipv4_gateway` must be omitted or left null. For a static address, set both values, for example:

```hcl
vm_ipv4_address = "192.168.1.50/24"
vm_ipv4_gateway = "192.168.1.1"
```

DHCP is used initially because it avoids guessing the LAN's addressing scheme. NetBird later provides a separate address for private access to Immich; that NetBird address is not the same as the VM's LAN address.

## Proxmox snippets

A Proxmox snippet is a small text file stored on a Proxmox datastore, rather than a VM disk image. This project uploads a cloud-init YAML snippet and attaches it to the VM as custom user data.

We use a snippet because the VM needs more than Proxmox's basic cloud-init fields: it creates the Ubuntu user, installs Docker and NetBird, writes the Immich configuration, and starts the containers. Terraform renders and manages the file so VM bootstrap is repeatable.

The selected `snippet_storage` must support the `Snippets` content type. The rendered snippet can contain sensitive values, so protect Terraform state and revoke short-lived setup credentials after enrollment.

Set these in untracked `terraform.tfvars`:

| Variable | Purpose |
|---|---|
| `proxmox_api_url` | Proxmox API endpoint. |
| `proxmox_insecure_tls` | Permit self-signed TLS; prefer `false` when possible. |
| `proxmox_node` | Target Proxmox node. |
| `vm_storage` | VM and cloud-init disk storage. |
| `image_storage` | Ubuntu cloud-image storage. |
| `snippet_storage` | Storage with Proxmox Snippets enabled. |
| `network_bridge` | Proxmox bridge, normally `vmbr0`. |
| `vm_ipv4_address` | VM address, normally `dhcp`. |
| `vm_ipv4_gateway` | Gateway for a static address; omit for DHCP. |
| `admin_username` | Ubuntu administrator name. |
| `ssh_public_key` | SSH public key; keep the private key on the workstation. |
| `immich_version` | Pinned Immich release tag. |
| `timezone` | Immich timezone. |

## Proxmox SSH access for snippet uploads

Cloud-init snippets use the provider's SSH upload path. The Terraform workstation must be able to log in to the Proxmox node with the configured SSH user and an SSH-agent key:

```bash
ssh-add ~/.ssh/id_ed25519
ssh-copy-id -i ~/.ssh/id_ed25519.pub root@<proxmox-netbird-ip>
ssh -o PreferredAuthentications=publickey -o PasswordAuthentication=no root@<proxmox-netbird-ip>
```

The configured `proxmox_ssh_address` is the Proxmox node's reachable SSH address, such as its NetBird address. Terraform does not use the workstation's `~/.ssh/config`; it uses the explicit provider address and SSH agent.

## Proxmox permissions

The Terraform API token uses separate roles with narrow scopes:

### `TerraformStorage`

Assign this role to the token on the datastore paths:

- `/storage/local`: `Datastore.Audit`, `Datastore.Allocate`, `Datastore.AllocateSpace`, and `Datastore.AllocateTemplate`
- `/storage/local-lvm` (or the configured `vm_storage`): `Datastore.Audit`, `Datastore.Allocate`, and `Datastore.AllocateSpace`

These permissions allow Terraform to inspect storage, upload the cloud-init snippet, download the Ubuntu image, and allocate VM disks.

The `local` datastore must also have the Proxmox `Snippets` content type enabled: `Datacenter > Storage > local > Edit > Content > Snippets`.

### `TerraformSys`

Assign this role to the token on `/nodes/pve` with:

- `Sys.Audit`
- `Sys.AccessNetwork`

`Sys.Audit` lets the provider discover the node endpoint. `Sys.AccessNetwork` allows Proxmox to query metadata for the Ubuntu download URL.

### `TerraformVM`

Assign this role to the token on `/vms` with:

- `VM.Allocate`
- `VM.Audit`
- `VM.Config.CPU`
- `VM.Config.Cloudinit`
- `VM.Config.Disk`
- `VM.Config.HWType`
- `VM.Config.Memory`
- `VM.Config.Network`
- `VM.Config.Options`
- `VM.PowerMgmt`
- `VM.GuestAgent.Audit`
- `VM.GuestAgent.Unrestricted`

These allow Terraform to create, configure, start, and read the VM through the QEMU guest agent, including retrieving its network interfaces and IP addresses.

### Network bridge access

Add `SDN.Use` to the VM role and assign it to the bridge's SDN path:

```text
/sdn/zones/localnetwork/vmbr0
```

If Proxmox only offers the parent path, use `/sdn/zones/localnetwork`. This allows the VM to attach its virtual NIC to `vmbr0` without granting broader SDN administration privileges.

After changing permissions or storage content types, create a fresh plan before applying:

```bash
terraform plan -out=immich.tfplan
terraform apply immich.tfplan
```

Environment-only secrets:

```bash
export PROXMOX_VE_API_TOKEN='terraform@pam!homelab=token-secret'
export NETBIRD_SETUP_KEY='short-lived-netbird-setup-key'
export IMMICH_DATABASE_PASSWORD='long-random-database-password'
```

These must not be placed in `terraform.tfvars`. The external data source also requires `jq` on the Terraform workstation. After provider changes, run `terraform init` and `terraform validate` from `terraform/`.

## Successful deployment checks

The initial VM deployment was verified by confirming:

- Cloud-init completed with `status: done`.
- NetBird enrolled the VM and created `wt0`.
- Immich, PostgreSQL, Redis, and machine-learning containers were running.
- Test images uploaded from both a phone and a laptop over NetBird.

Useful VM-side checks are:

```bash
cloud-init status --long
sudo netbird status
ip addr show wt0
cd /opt/immich && docker compose ps
```

If cloud-init reports a `scripts_user` error, inspect `/var/log/cloud-init-output.log`. A `setup key is invalid` message means NetBird enrollment failed; create a new short-lived setup key and recreate the VM so cloud-init receives the new value. Terraform's IP output comes from the QEMU guest agent and may not include the NetBird address until a later refresh.
