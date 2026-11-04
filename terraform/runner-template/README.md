# Runner template Terraform

This directory creates a temporary Ubuntu 24.04 builder for the ephemeral GitHub Actions runner template. It replaces the disposable Immich Terraform.

Terraform owns the repeatable builder definition. After boot, validate Docker and run /usr/local/sbin/prepare-runner-template as root. That script removes runner credentials/workspace state, Docker build state, machine identity, and cloud-init state. Shut down the VM and manually convert it to a Proxmox template.

The image contains generic tools only: Git, curl, CA certificates, jq, tar, unzip, QEMU guest agent, Docker Engine, Buildx, Compose, and an empty /opt/actions-runner directory. It contains no GitHub registration, App key, Proxmox credential, checkout, or application data.

It also grants the runner controller `VM.Clone` on the template (`proxmox_acl.template_clone`). Proxmox deletes that grant whenever the template VM is destroyed, so Terraform recreates it with every rebuild. The user and role it refers to come from `terraform/proxmox-access/`; apply that first.
