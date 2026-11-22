# Proxmox host firewall

Firewall rules for pve itself, so that other devices on the LAN (phones, guests, IoT) cannot reach its web UI. Without them the only protection is the login. Enabled on 2026-11-03.

| Source | Allowed to pve |
|---|---|
| Anyone | 22, SSH. Key-only, so there is no password to guess; see [step 1](#1-ssh-key-only). |
| Admin workstation's NetBird address, on `wt0` | 8006, web UI and API |
| Controller, on `vmbr1` | 8006, API |
| Anyone | UDP 51820, NetBird's WireGuard. It authenticates every packet. |
| Everything else | Dropped |

Rules match by NetBird address and by interface, never by LAN address, because the Xfinity gateway cannot reliably reserve DHCP addresses. Replies to pve's own connections (apt, NetBird's management servers, DNS) are always accepted. Terraform reaches pve over NetBird (`proxmox_api_url` is pve's NetBird address), so it is unaffected.

How Proxmox builds the host rules (from `pve-firewall`'s `Firewall.pm`): host rules come first, then datacenter rules, then built-in accepts for 22, 8006, VNC, SPICE and migration ports from the `management` IP set, then the datacenter input policy (`policy_in`, `DROP` when unset). Proxmox always adds `local_network`, the whole LAN, to `management`. So the last rule here drops those ports explicitly; leaving them to the input policy would keep the LAN open.

Guests are unaffected: these rules filter traffic *to pve*, and VM and container traffic is filtered by their own firewalls (`docs/runner-isolation.md`).

## Ways in

- **Normally:** NetBird.
- **NetBird down, at home:** SSH over the LAN, and tunnel the web UI through it: `ssh -L 8006:localhost:8006 root@<pve LAN IP>`, then open `https://localhost:8006`.
- **Locked out entirely:** plug a keyboard and monitor into pve, log in as root, and run `pvesh set /nodes/pve/firewall/options --enable 0`. That turns the host firewall off; VM firewalls keep working.

## Rollout

The order matters: every path that will stay open must work before the firewall closes the others. Keep two root sessions on pve open throughout, one over NetBird (`ssh root@<pve NetBird IP>`) and one over the LAN (`ssh root@<pve LAN IP>`), so that losing one path never locks you out.

### 1. SSH key-only

First confirm key login works on its own, from the workstation; it must print `key-ok`:

```sh
ssh -o PreferredAuthentications=publickey -o PasswordAuthentication=no root@<pve LAN IP> true && echo key-ok
```

Then on pve:

```sh
printf 'PasswordAuthentication no\nKbdInteractiveAuthentication no\nPermitRootLogin prohibit-password\n' > /etc/ssh/sshd_config.d/10-key-only.conf
sshd -t && systemctl reload ssh
```

`sshd -T | grep -E 'passwordauthentication|kbdinteractive|permitrootlogin'` shows `no`, `no` and `without-password` (sshd's name for `prohibit-password`). That reads the config, not the running daemon; the real test is from a new workstation terminal: `ssh -o PubkeyAuthentication=no root@<pve LAN IP>` must be refused without a password prompt. Terraform's `runner-template` root logs in with ssh-agent keys and is unaffected.

### 2. Controller bridge

The controller reaches the API over `vmbr1`, a bridge with no physical port that only pve (`10.10.10.1`) and the controller (`10.10.10.2`) are on. Both addresses are static, so the rule never depends on DHCP. On pve:

```sh
cp /etc/network/interfaces /etc/network/interfaces.bak-$(date +%F)
printf '\nauto vmbr1\niface vmbr1 inet static\n    address 10.10.10.1/24\n    bridge-ports none\n    bridge-stp off\n    bridge-fd 0\n' >> /etc/network/interfaces
ifreload -a
```

`ip -br addr show vmbr1` shows `10.10.10.1/24` (state `UNKNOWN` is normal for a bridge with no physical port), and `vmbr0` is unchanged. To undo, copy the backup back and `ifreload -a`.

### 3. Move the controller onto it

`terraform/controller/` adds `eth1` on `vmbr1` and changes the container firewall's API destination to `10.10.10.1`. From then on the controller cannot reach `<pve LAN IP>`.

1. Check no job is running (`pvesh get /pools/gha-runners --output-format json | grep -c vmid` is `0`), then `pct exec 920 -- systemctl stop gha-controller`.
2. Remove any `proxmox_api_address` line from `terraform/controller/terraform.tfvars`; it would override the new default.
3. Grant the Terraform token `SDN.Use` on both bridges (see `terraform/controller/README.md`). Updating the container re-sends every NIC, so without it the apply fails with `403 Permission check failed (/sdn/zones/localnetwork/vmbr0, SDN.Use)`.
4. `terraform plan` shows `0 to add, 2 to change, 0 to destroy`: the container in place and its firewall rules. Stop on any replacement; replacing the container wipes its credentials. Then `terraform apply`.
5. The apply reboots the container, and the service starts again by itself with the old URL (`dial tcp <pve LAN IP>:8006: i/o timeout` in its log). That is expected until the next step.

Check on pve: `pct config 920 | grep ^net` lists `net1` with `bridge=vmbr1,ip=10.10.10.2/24,firewall=1`, and `pct exec 920 -- ip -br addr show eth1` shows the address.

Then point the controller at pve by name. The node certificate lists `DNS:pve` but only the LAN IP, so the name resolves to the `vmbr1` address in the container's `/etc/hosts`:

```sh
pct exec 920 -- sh -c 'grep -q " pve$" /etc/hosts || echo "10.10.10.1 pve" >> /etc/hosts'
pct exec 920 -- curl -sS -o /dev/null -w '%{http_code}\n' https://pve:8006/api2/json/version
pct exec 920 -- sed -i 's|^PROXMOX_API_URL=.*|PROXMOX_API_URL=https://pve:8006|' /etc/gha-controller/controller.env
pct exec 920 -- systemctl restart gha-controller
```

The `curl` must print `401`: reached pve, certificate verified, not logged in. A certificate error means the CA or the name is wrong; a timeout means `vmbr1` or the container firewall. Use `restart`, not `start`: the service is already running and only reads `controller.env` when it starts. Then `journalctl -u gha-controller` shows polling with no errors, and the **Runner smoke test** workflow passes. The host firewall is still off at this point, so this proves the new path before anything is blocked.

### 4. Checks

On pve:

```sh
cat /etc/pve/firewall/cluster.fw
pvesh get /nodes/pve/firewall/options
pvesh get /nodes/pve/firewall/rules
ip -br addr show wt0
netbird status | grep -i 'wireguard port'
```

- `cluster.fw`: `policy_in` must be `DROP` or absent. `pvesh get /cluster/firewall/options` fails with `unknown schema type (not an object?)`, a `pvesh` bug ([#7942](https://lore.proxmox.com/all/l2dfgczwqkgq4bkyubvijobmg5xkdeqnr6rpkqy2g3d4yjbk35@ycmgqof4ukiv/t/)) in schemas without properties; node paths are unaffected.
- Node options: `enable 0`. Node rules: empty, or Terraform would replace them.
- `wt0` holds pve's NetBird address, and NetBird listens on `51820`.

### 5. Write the rules

The Terraform token (`root@pam!terraform`, privilege-separated) needs `Sys.Audit` and `Sys.Modify` on `/nodes/pve` (the Proxmox API schema for `GET`/`POST /nodes/{node}/firewall/rules`). `Sys.Modify` on a node also covers its network, DNS, time and package settings, so this is close to root on pve; the token is already guarded like root. On pve:

```sh
pveum role add TerraformNode --privs "Sys.Audit,Sys.Modify"
pveum acl modify /nodes/pve --tokens 'root@pam!terraform' --roles TerraformNode
```

On the workstation (`admin_netbird_ip` from `netbird status`):

```sh
cd terraform/proxmox-firewall
cp terraform.tfvars.example terraform.tfvars
terraform init
terraform plan
terraform apply
```

The plan adds one resource with 5 rules. Applying only writes them: `cat /etc/pve/nodes/pve/host.fw` lists them in order under `[RULES]`, with `enable: 0` under `[OPTIONS]`.

### 6. Enable

Established connections survive enabling the firewall. Arm an undo timer first, so a mistake reverts itself in ten minutes. On pve:

```sh
systemd-run --on-active=10min --unit=hostfw-undo pvesh set /nodes/pve/firewall/options --enable 0
pvesh set /nodes/pve/firewall/options --enable 1
pve-firewall status
```

Run the checks in [Verify](#verify). If they all pass, cancel the undo with `systemctl stop hostfw-undo.timer`; `pvesh get /nodes/pve/firewall/options` then stays at `enable 1`. If one fails, run `pvesh set /nodes/pve/firewall/options --enable 0` or wait for the timer.

## Verify

From the workstation, each in a new connection:

```sh
nc -vz -w 5 <pve NetBird IP> 8006
nc -vz -w 5 <pve NetBird IP> 22
nc -vz -w 5 <pve LAN IP> 22
nc -vz -w 5 <pve LAN IP> 8006
```

The first three succeed; the last times out. From a phone on the Wi-Fi, `https://<pve LAN IP>:8006` times out too. On pve, `pct exec 920 -- curl -sS -o /dev/null -w '%{http_code}\n' https://pve:8006/api2/json/version` still prints `401`, and the **Runner smoke test** workflow passes.
