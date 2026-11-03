# Ephemeral runner controller

The controller serves queued GitHub Actions jobs labeled `homelab-ephemeral` with one fresh Proxmox VM per job. It polls an allowlist of up to five repositories, picks the job that has waited longest, clones the runner template into the `gha-runners` pool, hands the VM a just-in-time runner config through the guest agent, waits for the job, and always destroys the VM.

The GitHub App permissions and the scoped Proxmox token are documented, with the API call behind each privilege, in [docs/runner-controller-access.md](../docs/runner-controller-access.md). No controller credential belongs in the golden image or workflow secrets.

## Usage

```sh
cp config.example.env ~/.config/homelab/controller.env   # fill in, then chmod 600
go build -o controller .
./controller once -dry-run   # log cleanup and the planned VM; changes nothing
./controller once            # clean up, then serve at most one queued job
./controller run             # poll every POLL_INTERVAL_SECONDS until Ctrl-C
```

`run` keeps serving one job at a time and logs and retries failed polls. Ctrl-C during a job still deregisters the runner and destroys the VM before exiting.

## Runner logs

If a job fails after the VM booted (the runner never comes online, the job times out, or Ctrl-C), the controller copies the runner service's output (`/var/log/gha-runner.log` in the VM) through the guest agent before destroying the VM, and saves it as `<runner name>.log` in `RUNNER_LOG_DIR` (default `~/.local/state/homelab/runner-logs`). Successful jobs keep their logs on GitHub. A controller crash mid-job loses that VM's log, since reconcile destroys it later without reading it.

## Recovering from crashes

Every poll (and every `once`) first reconciles the pool against GitHub, so a crash, a sleeping laptop, or a failed cleanup is repaired on the next start. Runner VM names carry their creation time (`gha-runner-<vmid>-<unix>`), which lets the controller judge age without any local state:

- A VM whose runner is no longer registered is destroyed once it is older than the setup window (agent wait + runner-online wait, 8 minutes). Its job finished, or it never got one.
- A VM older than the maximum lifetime (setup + job timeout + cleanup, 73 minutes) is destroyed even if its runner is still registered.
- A `gha-runner-` registration whose VM is no longer in the pool is removed.
- Younger VMs are left alone, because they may still be setting up or running a job.

Only one controller should run at a time; two would compete for the same jobs.

## Guardrails

- Clones only from `PROXMOX_RUNNER_TEMPLATE_VMID`, which must be outside the runner ID range.
- Runner VMs use IDs from `PROXMOX_RUNNER_VMID_RANGE` and names starting with `gha-runner-`; a VM that fails either check is never stopped or destroyed.
- Refuses to clone while `PROXMOX_MAX_POOL_VMS` VMs are already in the pool, and stops if the pool contains anything that is not a runner.
- The runner is registered only after the VM's guest agent answers, so a failed boot leaves nothing on GitHub.
- Cleanup (deregister, stop, destroy) runs on every outcome, including timeouts and Ctrl-C, with its own deadline.
- The config file is rejected unless only its owner can read it.

## Layout

One package, split by concern: `config.go`, `github.go` (GitHub App and Actions API), `proxmox.go` (minimal VE API client), and `runner.go` (the lifecycle, written against small interfaces so `runner_test.go` can drive it with fakes).
