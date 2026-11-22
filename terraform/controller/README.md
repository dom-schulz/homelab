# Controller container

An unprivileged Ubuntu 24.04 LXC container (VMID 920 by example) that runs the runner controller as a systemd service. It is its own Terraform root, apart from `terraform/runner-template/`, so template rebuilds never plan against the controller.

Terraform creates only the container and its firewall. Nothing secret passes through Terraform: `scripts/deploy-controller.sh` installs the binary and copies the two credentials in through pve.

## Firewall

- In: SSH from `ssh_sources` only; everything else dropped.
- Out: DNS and the Proxmox API (`proxmox_api_address:8006`) are the only local destinations. The API is reached over `eth1` on `vmbr1`, a bridge only pve and the controller are on, so pve's host firewall can allow it by interface (`terraform/proxmox-firewall/README.md`). Other private, NetBird and link-local IPv4 is dropped and IPv6 rejected, as for runner VMs. The internet (GitHub, Ubuntu mirrors) is allowed.

The controller reaches runner VMs only through the Proxmox API (guest agent), never over the network.

## Create

Uses the same Proxmox credentials as `terraform/runner-template/`. Creating or updating the container attaches its NICs, which needs `SDN.Use` on each bridge; the privilege-separated Terraform token (`root@pam!terraform`) gets it from the built-in `PVESDNUser` role (`SDN.Audit`, `SDN.Use`). Without it, the apply fails with `403 Permission check failed (/sdn/zones/localnetwork/vmbr0, SDN.Use)`. On pve, once:

```sh
for b in vmbr0 vmbr1; do pveum acl modify /sdn/zones/localnetwork/$b --tokens 'root@pam!terraform' --roles PVESDNUser; done
```

Root's SSH keys come from `admin-keys.pub` at the repository root (copy `admin-keys.pub.example`), the same list `scripts/sync-pve-admin-keys.sh root@<pve>` puts on pve, so one edit covers both hosts. Proxmox only applies keys when it creates a container, so changing the list recreates the controller; back up `/etc/gha-controller` first and rerun scripts/deploy-controller.sh.

```sh
cd terraform/controller
cp terraform.tfvars.example terraform.tfvars   # fill in
terraform init
terraform plan    # expect: template download, container, firewall options and rules
terraform apply
```

## Deploy

Keep the two credentials on your workstation in `~/.config/homelab/` (mode 600):

- `controller.env`, from `controller/config.example.env`, with `GITHUB_APP_PRIVATE_KEY_FILE=/etc/gha-controller/github-app.pem`, `PROXMOX_API_URL=https://pve:8006` and `PROXMOX_TLS_INSECURE=false`.
- `github-app.pem`, the GitHub App private key.

Then, with SSH access to pve:

```sh
scripts/deploy-controller.sh root@<pve address>
```

It cross-compiles a static binary and installs it in container 920 with `pct push` and `pct exec`, so it never needs the container's own address. Rerun it to ship a new version; a restart stops any in-flight job. On a fresh container it also creates the `gha-controller` user and `/etc/gha-controller`, and copies in the credentials; credentials already there are never overwritten, so to change one, edit it in the container and `systemctl restart gha-controller`. Stop any controller running elsewhere first; two would compete for jobs.

The controller verifies the Proxmox API certificate, so the container must trust the Proxmox cluster CA. Without verification, any host on the LAN, runner VMs included, could impersonate pve with ARP spoofing and receive the token. pve's node certificate lists `DNS:pve` but only the LAN IP (check with `openssl x509 -in /etc/pve/local/pve-ssl.pem -noout -ext subjectAltName`), so the controller uses the name and resolves it to pve's `vmbr1` address. The script installs `/etc/pve/pve-root-ca.pem` and adds that hosts entry on every run; Proxmox rewrites only its own marked section of a container's `/etc/hosts`, so the line stays.

## Operate

```sh
journalctl -u gha-controller -f              # controller log
ls /var/lib/gha-controller/runner-logs       # logs of failed runner VMs
systemctl restart gha-controller             # after editing controller.env
```

The container is not updated automatically; run `apt update && apt upgrade` on it now and then.
