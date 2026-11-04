# Controller container

An unprivileged Ubuntu 24.04 LXC container (VMID 920 by example) that runs the runner controller as a systemd service. It is its own Terraform root, apart from `terraform/runner-template/`, so template rebuilds never plan against the controller.

Terraform creates only the container and its firewall. Nothing secret passes through Terraform or Proxmox: the binary is installed by `scripts/deploy-controller.sh`, and the two credentials are copied by hand.

## Firewall

- In: SSH from `ssh_sources` only; everything else dropped.
- Out: DNS and the Proxmox API (`proxmox_api_address:8006`) are the only local destinations. Other private, NetBird and link-local IPv4 is dropped and IPv6 rejected, as for runner VMs. The internet (GitHub, Ubuntu mirrors) is allowed.

The controller reaches runner VMs only through the Proxmox API (guest agent), never over the network.

## Create

Uses the same Proxmox credentials as `terraform/runner-template/`.

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
  #   PROXMOX_API_URL=https://<Proxmox LAN address>:8006   (NetBird is not reachable from here)
  #   RUNNER_LOG_DIR=/var/lib/gha-controller/runner-logs
  #   PROXMOX_TLS_INSECURE=false
  systemctl start gha-controller
```

The controller verifies the Proxmox API certificate, so the container must trust the Proxmox cluster CA. Without verification, any host on the LAN, runner VMs included, could impersonate pve with ARP spoofing and receive the token. pve's node certificate must list the address in `PROXMOX_API_URL`; check with `openssl x509 -in /etc/pve/local/pve-ssl.pem -noout -ext subjectAltName` on pve. Install the CA before starting the service, and again if it is ever regenerated. On pve:

```sh
pct push <controller vmid> /etc/pve/pve-root-ca.pem /usr/local/share/ca-certificates/pve-root-ca.crt
pct exec <controller vmid> -- update-ca-certificates
```

## Operate

```sh
journalctl -u gha-controller -f              # controller log
ls /var/lib/gha-controller/runner-logs       # logs of failed runner VMs
systemctl restart gha-controller             # after editing controller.env
```

The container is not updated automatically; run `apt update && apt upgrade` on it now and then.
