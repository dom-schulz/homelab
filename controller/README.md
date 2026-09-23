# Ephemeral runner controller

The controller is intentionally not wired to GitHub or Proxmox yet. GitHub App registration and Proxmox API credentials are operator-controlled setup steps.

The planned outbound-only controller will poll queued jobs labeled homelab-ephemeral, request a short-lived just-in-time runner configuration, clone one VM from the trusted template, inject one-time bootstrap data, wait for registration and completion, then quarantine or destroy the VM.

It must enforce an explicit template/pool/name-prefix allowlist, a maximum concurrency of one, durable lifecycle logs outside the runner, restart reconciliation, bounded stale-VM cleanup, and dry-run mode before create/delete operations. No controller credential belongs in the golden image or workflow secrets.
