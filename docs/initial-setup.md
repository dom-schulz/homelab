# Initial setup

This document records the first working homelab milestone and the current direction. It is a personal learning log, not a complete reproduction guide.

## Starting point

I repurposed an HP ENVY x360 laptop with a broken internal display as a headless Proxmox server. The host is reachable over the local network and NetBird. The system has limited resources, so the platform is intentionally small and disposable.

## Current direction

The Immich experiment proved that the VM can host self-managed workloads. Immich Terraform is no longer the target architecture and can be removed when the runner platform is ready; preserve any data first if that ever changes.

The next platform milestone is:

- a Terraform/cloud-init Ubuntu runner-template builder;
- a manually validated Proxmox golden template;
- a future outbound-only controller that creates one fresh VM per GitHub Actions job;
- Docker builds pushed to GHCR using immutable commit-SHA tags;
- a small k3s application and external monitoring stack later.

See terraform/runner-template/README.md and plans/ephemeral-github-runners.md for the implementation boundary and decisions.
