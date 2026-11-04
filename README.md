# Homelab

This repository is a personal record of my homelab experiments: Proxmox, networking, infrastructure as code, Kubernetes, observability, and disposable CI runners.

It is a learning journal rather than a production blueprint. The hardware is intentionally constrained, so designs favor small, inspectable components and explicit failure handling.

## Current direction

The first working Immich VM validated that this Proxmox host can run self-hosted services. The current build-out replaces that one-off application Terraform with a reproducible runner-template foundation:

- Terraform and cloud-init build a temporary Ubuntu 24.04 runner-template VM.
- Docker Engine, Buildx, Compose, and generic CI prerequisites are baked into the template.
- A future trusted controller will clone one fresh VM per GitHub Actions job.
- Images will be built on the disposable runner and pushed to public GHCR by immutable commit-SHA tag.
- k3s and Grafana/Prometheus remain later workload and observability exercises.

## Working notes

- Initial setup: docs/initial-setup.md
- Runner template Terraform: terraform/runner-template/README.md
- Controller container: terraform/controller/README.md
- Proxmox users, roles and ACLs: terraform/proxmox-access/README.md
- Ephemeral runner plan: plans/ephemeral-github-runners.md

The repository intentionally does not contain GitHub App credentials, Proxmox credentials, Terraform state, or runner registration tokens.
