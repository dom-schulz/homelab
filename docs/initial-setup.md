# Initial setup

This document records my first working milestone while building the homelab. It is a personal learning log, not a complete recipe for reproducing my home network.

## Starting point

I repurposed an HP ENVY x360 laptop with a broken internal display as the server. I installed Proxmox VE using an automated installer image because the display could not be relied on during installation.

The server has limited resources, so the initial goal is to learn the workflow with a small number of services rather than build some large production platform immediately.

## Problems I worked through

- The internal display fades to black after boot, while the operating system continues running.
- The external display did not provide a dependable installation interface.
- Wi-Fi had to be prepared before the machine could be managed headlessly.
- A USB Ethernet adapter initially had connection problems, but wired networking became available after reconnecting it.
- Closing the lid powered the laptop down. I changed the systemd logind lid behavior so the server can run with the lid closed.
- Because the machine is headless, SSH is the primary administration method as needed.

## Current milestone

The following are working:

- Proxmox VE boots from the laptop's internal storage.
- The Proxmox host is reachable over the local network.
- The host is reachable over NetBird.
- This laptop and my phone can connect to the Proxmox host through NetBird.
- The installation USB is no longer required for normal boot.

Host addresses, NetBird names, account identifiers, fingerprints, and other environment-specific values are intentionally omitted.

## Lessons learned

The display problem made a black screen misleading at first, it did not necessarily mean that Ubuntu or Proxmox had failed. Network connectivity, SSH, and boot logs were better indicators of whether the machine was actually running.

I also learned to identify disks carefully before mounting or writing to them. Device names can change depending on which computer or USB device is connected, so stable identifiers such as filesystem UUIDs are safer than assuming a device name will remain the same.

## Next learning goals

1. Keep the Proxmox host reachable and manageable over NetBird.
2. Create a small test VM using infrastructure as code.
3. Install and test Immich in the VM before purchasing dedicated storage.
4. Document decisions, failures, and lessons learned as the setup evolves.
