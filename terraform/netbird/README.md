# NetBird Terraform

This separate stack manages the NetBird Cloud control plane for the homelab.

Terraform manages single-peer groups, the homelab Network, the runner resource (`runner_address`), the Proxmox routing peer, and TCP 22/8006 policies. It does not install NetBird, enroll peers, configure Proxmox networking, enable Linux forwarding, or manage ephemeral runner peers.

## Before the first run

1. Create a NetBird Management API token with the minimum permissions needed for groups, networks, resources, routers, and policies.
2. Add it to the environment as `NB_PAT`; the provider reads this automatically.
3. Confirm the existing peer names are exactly `dschulz-HP-Pavilion` and `proxmox-host`; Terraform looks them up through data sources.
4. Copy `terraform.tfvars.example` to `terraform.tfvars` only if you need to override the runner address.
5. Import or reconcile the existing broad workstation-to-Proxmox policy before applying the narrowed policies.

Do not commit `terraform.tfvars` or state files.

## Manual prerequisites

`proxmox-host` must already be enrolled and able to reach the runner address. The host may require IP forwarding and firewall forwarding rules. Masquerading is enabled initially so the runner does not need a return route to the NetBird overlay.

## Safe workflow

Run manually from this directory:

```bash
terraform init
terraform fmt -check
terraform validate
terraform plan
```

Review the plan before applying it. Confirm the old unrestricted policy is imported, narrowed, or removed as intended.

## Validation

```bash
netbird status -d
netbird networks ls
ip route get <runner IP>
nc -vz -w 5 <proxmox-host-address> 22
nc -vz -w 5 <proxmox-host-address> 8006
nc -vz -w 5 <runner IP> 22
```

Unrelated ports should remain inaccessible. The iPhone is not assigned to the administrative group.
