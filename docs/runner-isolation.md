# Runner network isolation

Runner VMs execute workflow code with Docker access, so they are treated as untrusted. They sit on `vmbr0` with everything else, and the Proxmox firewall keeps them off the local network:

- Inbound: everything dropped, except DHCP and replies to the VM's own connections. Nothing needs to connect in; the controller talks to the guest agent over a virtio serial device, not the network.
- Outbound: DNS to the LAN resolver is allowed; private (`10/8`, `172.16/12`, `192.168/16`), NetBird/CGNAT (`100.64/10`) and link-local IPv4 destinations are dropped; all IPv6 is rejected. The rest of IPv4, the internet (GitHub, GHCR), is allowed.

IPv6 is rejected outright because LAN hosts, the Proxmox host included, also have global IPv6 addresses that no private-range rule would catch, and blocking only the LAN's /64 would silently break when the ISP changes the prefix. Rejecting (rather than dropping) makes clients fall back to IPv4 immediately.

The rules live on template 910 (`terraform/main.tf`); `qm clone` copies a VM's firewall config, so every runner inherits them. A future deploy target (the k3s API) gets one `ACCEPT` rule placed before the drops.

## Enabling the firewall (operator, once)

Proxmox ignores VM rules until the datacenter firewall is on, and turning it on also enables the host firewall (`enable` defaults to 1), whose default rules allow port 8006/22 only from the auto-detected `local_network`. Access over NetBird would be cut. To avoid that, the host firewall is disabled explicitly first; the host's own rules can be designed separately later.

Keep a root shell on pve open (the web UI shell or `ssh root@<LAN IP>`) while doing this.

```sh
pvesh set /nodes/pve/firewall/options --enable 0     # host firewall off: host traffic unaffected
pvesh set /cluster/firewall/options --enable 1       # datacenter on: VM rules can apply
pve-firewall status                                  # expect enabled/running
```

Then check the web UI still loads over NetBird. VMs whose NIC has `firewall=0` are unaffected.

To undo: `pvesh set /cluster/firewall/options --enable 0`.

## Applying the template rules

1. Optionally set `runner_dns_servers` in `terraform.tfvars` if the LAN resolver is not the default.
2. `terraform plan`: expect an in-place update of `runner_template_builder` (`firewall = true` on the NIC) and two new firewall resources. Stop if it shows a replacement.
3. `terraform apply`, then on pve: `cat /etc/pve/firewall/910.fw` and `grep net0 /etc/pve/qemu-server/910.conf` (expect `firewall=1`).

## Verifying

Run the **Runner smoke test** workflow with the controller (`controller once`). Its "Check network isolation" step passes only if DNS and the internet work while the Proxmox API (LAN and NetBird), SSH on pve and the gateway's admin ports are unreachable. While the job runs, `cat /etc/pve/firewall/<vmid>.fw` on pve should match the template's.
