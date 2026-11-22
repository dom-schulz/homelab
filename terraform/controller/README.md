# Controller container

An unprivileged Ubuntu 24.04 LXC container (VMID 920 by example) that runs the runner controller as a systemd service. It is its own Terraform root, apart from `terraform/runner-template/`, so template rebuilds never plan against the controller.

Terraform creates only the container and its firewall. Nothing secret passes through Terraform or Proxmox: the binary is installed by `scripts/deploy-controller.sh`, and the two credentials are copied by hand.

## Firewall

- In: SSH from `ssh_sources` only; everything else dropped.
- Out: DNS and the Proxmox API (`proxmox_api_address:8006`) are the only local destinations. The API is reached over `eth1` on `vmbr1`, a bridge only pve and the controller are on, so pve's host firewall can allow it by interface (`terraform/proxmox-firewall/README.md`). Other private, NetBird and link-local IPv4 is dropped and IPv6 rejected, as for runner VMs. The internet (GitHub, Ubuntu mirrors) is allowed.

The controller reaches runner VMs only through the Proxmox API (guest agent), never over the network.

## Create

Uses the same Proxmox credentials as `terraform/runner-template/`. Creating or updating the container attaches its NICs, which needs `SDN.Use` on each bridge; the privilege-separated Terraform token (`root@pam!terraform`) gets it from the built-in `PVESDNUser` role (`SDN.Audit`, `SDN.Use`). Without it, the apply fails with `403 Permission check failed (/sdn/zones/localnetwork/vmbr0, SDN.Use)`. On pve, once:

```sh
for b in vmbr0 vmbr1; do pveum acl modify /sdn/zones/localnetwork/$b --tokens 'root@pam!terraform' --roles PVESDNUser; done
```

Root's SSH keys come from `admin-keys.pub` at the repository root (copy `admin-keys.pub.example`), the same list `scripts/sync-pve-admin-keys.sh root@<pve>` puts on pve, so one edit covers both hosts. Proxmox only applies keys when it creates a container, so changing the list recreates the controller; back up `/etc/gha-controller` first and redo Deploy and Credentials below.

```sh
cd terraform/controller
cp terraform.tfvars.example terraform.tfvars   # fill in
terraform init
terraform plan    # expect: template download, container, firewall options and rules
terraform apply
```

## Deploy

```sh
scripts/deploy-controller.sh root@<controller address>   # terraform output ipv4, or on pve: pct exec 920 -- ip -4 addr show eth0
```

It cross-compiles a static binary, installs it as `/usr/local/bin/gha-controller` with `controller/deploy/gha-controller.service`, and on first run creates the `gha-controller` system user and `/etc/gha-controller`. Rerun it to ship a new version. A restart stops any in-flight job.

## Credentials (once)

Stop any controller running elsewhere first; two would compete for jobs.

```sh
scp ~/.config/homelab/controller.env root@<controller>:/etc/gha-controller/controller.env
scp <path to GitHub App .pem> root@<controller>:/etc/gha-controller/github-app.pem
ssh root@<controller>
  cd /etc/gha-controller
  chown gha-controller:gha-controller controller.env github-app.pem
  chmod 600 controller.env github-app.pem
  # Then edit controller.env:
  #   GITHUB_APP_PRIVATE_KEY_FILE=/etc/gha-controller/github-app.pem
  #   PROXMOX_API_URL=https://pve:8006   (pve over vmbr1, via the hosts entry below)
  #   RUNNER_LOG_DIR=/var/lib/gha-controller/runner-logs
  #   PROXMOX_TLS_INSECURE=false
  systemctl start gha-controller
```

The controller verifies the Proxmox API certificate, so the container must trust the Proxmox cluster CA. Without verification, any host on the LAN, runner VMs included, could impersonate pve with ARP spoofing and receive the token. pve's node certificate must list the name in `PROXMOX_API_URL`; check with `openssl x509 -in /etc/pve/local/pve-ssl.pem -noout -ext subjectAltName` on pve. It lists `DNS:pve` but only the LAN IP, so the controller uses the name and resolves it to pve's `vmbr1` address. Proxmox rewrites only its own marked section of a container's `/etc/hosts`, so the line stays. Install the CA before starting the service, and again if it is ever regenerated. On pve:

```sh
pct push <controller vmid> /etc/pve/pve-root-ca.pem /usr/local/share/ca-certificates/pve-root-ca.crt
pct exec <controller vmid> -- update-ca-certificates
pct exec <controller vmid> -- sh -c 'echo "10.10.10.1 pve" >> /etc/hosts'
```

## Operate

```sh
journalctl -u gha-controller -f              # controller log
ls /var/lib/gha-controller/runner-logs       # logs of failed runner VMs
systemctl restart gha-controller             # after editing controller.env
```

The container is not updated automatically; run `apt update && apt upgrade` on it now and then.
