# Runner controller access

The controller holds two credentials: a GitHub App key and a Proxmox API token. Both are scoped to exactly the API calls the controller makes. Neither belongs in the repository, the runner template, runner VMs, or workflow secrets. Store them on the controller host only, readable by its user alone (`chmod 600`).

Every permission below was taken from the API documentation for the specific call that needs it: GitHub's "Permissions required for GitHub Apps" reference and the Proxmox API schema (`pve-docs/api-viewer`), checked against Proxmox VE 9.2.

## GitHub App

Created under the personal account, installed with **Only select repositories** (the allowlist, at most five). Webhooks are disabled because the controller polls.

| Repository permission | Access | Needed for |
|---|---|---|
| Actions | Read | `GET /repos/{owner}/{repo}/actions/runs` and `.../runs/{run_id}/jobs` to find queued `homelab-ephemeral` jobs |
| Administration | Read and write | `POST .../actions/runners/generate-jitconfig` (write), `GET .../actions/runners` (read), `DELETE .../actions/runners/{runner_id}` (write) |
| Metadata | Read | Added automatically by GitHub |

Administration write is broader than runner management (it also covers repository settings). GitHub offers no narrower permission for registering repository runners, so the private key must stay on the controller.

## Proxmox API token

A dedicated user `gha-controller@pve` with token `controller` (`privsep 0`, so the token has exactly the user's permissions). Runner clones live in the `gha-runners` pool; the template (VM 910) is deliberately **not** in that pool, so the token can clone it but not modify or delete it.

| Controller action | API call | Privilege required | Granted on |
|---|---|---|---|
| Clone the template | `POST /nodes/{node}/qemu/910/clone` | `VM.Clone` on the source | `/vms/910` |
| Place the clone in the pool | same call with `pool=gha-runners` | `VM.Allocate` on the pool | `/pool/gha-runners` |
| Allocate the clone's disk | same call | `Datastore.AllocateSpace` | `/storage/local-lvm` |
| Attach the clone's network | same call | `SDN.Use` | `/sdn/zones/localnetwork/vmbr0` |
| Drop inherited `cicustom`, set `ciupgrade=0` | `PUT .../qemu/{vmid}/config` | `VM.Config.Cloudinit` | `/pool/gha-runners` |
| Start and stop | `POST .../status/start`, `.../status/stop` | `VM.PowerMgmt` | `/pool/gha-runners` |
| Read VM status | `GET .../status/current` | `VM.Audit` | `/pool/gha-runners` |
| Wait for the guest agent | `POST .../agent/ping` | `VM.GuestAgent.Audit` | `/pool/gha-runners` |
| Hand over the runner config | `POST .../agent/file-write` | `VM.GuestAgent.FileWrite` | `/pool/gha-runners` |
| Copy out the runner log of a failed VM before destroying it | `GET .../agent/file-read` | `VM.GuestAgent.FileRead` | `/pool/gha-runners` |
| Destroy the clone | `DELETE .../qemu/{vmid}` | `VM.Allocate` | `/pool/gha-runners` |
| List runner VMs for reconciliation | `GET /pools/gha-runners` | `Pool.Audit` | `/pool/gha-runners` |
| Follow its own clone/start/stop tasks | `GET /nodes/{node}/tasks/{upid}/status` | none for tasks the token started | n/a |

Setup, run as root on the Proxmox host:

```sh
pveum pool add gha-runners --comment "Ephemeral GitHub Actions runner clones"

pveum role add GhaRunnerPool --privs "VM.Allocate,VM.Audit,VM.Config.Cloudinit,VM.PowerMgmt,VM.GuestAgent.Audit,VM.GuestAgent.FileWrite,VM.GuestAgent.FileRead,Pool.Audit"
pveum role add GhaTemplateClone --privs "VM.Clone"
pveum role add GhaRunnerStorage --privs "Datastore.AllocateSpace"
pveum role add GhaRunnerNetwork --privs "SDN.Use"

pveum user add gha-controller@pve --comment "Runner controller; no login"
pveum user token add gha-controller@pve controller --privsep 0 --comment "gha runner controller"

pveum acl modify /pool/gha-runners --users gha-controller@pve --roles GhaRunnerPool
pveum acl modify /vms/910 --users gha-controller@pve --roles GhaTemplateClone
pveum acl modify /storage/local-lvm --users gha-controller@pve --roles GhaRunnerStorage
pveum acl modify /sdn/zones/localnetwork/vmbr0 --users gha-controller@pve --roles GhaRunnerNetwork
```

Rebuilding the template (Terraform replaces VM 910) drops the `/vms/910` grant along with the old VM, and cloning then fails with `403 Permission check failed (/vms/910, VM.Clone)`. Re-grant it after every rebuild:

```sh
pveum acl modify /vms/910 --users gha-controller@pve --roles GhaTemplateClone
```

`pveum user token add` prints the secret once. The controller's `PROXMOX_API_TOKEN` is `gha-controller@pve!controller=<secret>`. To rotate or recover a lost secret:

```sh
pveum user token remove gha-controller@pve controller
pveum user token add gha-controller@pve controller --privsep 0 --comment "gha runner controller"
```

Verify the effective permissions from the controller host:

```sh
curl -sk -H "Authorization: PVEAPIToken=gha-controller@pve!controller=<secret>" \
  https://<pve>:8006/api2/json/access/permissions
```

The result should list only `/pool/gha-runners` (and the VMs in it), `/vms/910`, `/storage/local-lvm`, and `/sdn/zones/localnetwork/vmbr0`.

## Accepted breadth

These grants are wider than the controller's intent, but Proxmox has no narrower option:

- `Datastore.AllocateSpace` applies to all of `local-lvm`, which other VMs share. See [Shared storage](#shared-storage) for how runner space is bounded.
- `VM.GuestAgent.FileRead` can read any file inside runner VMs, not only the runner log. Those VMs hold nothing but the job, and the controller already created its credentials.
- `VM.GuestAgent.FileWrite` is root-equivalent inside runner VMs. That is acceptable because the controller owns those VMs; it has no effect outside the pool.
- `VM.Allocate` on the pool allows creating and destroying any VM in the pool. Nothing except runner clones may be added to `gha-runners`.

## Shared storage

`local-lvm` is one LVM thin pool (349 GiB when this was written) that holds every VM disk on the host: the template, runner clones, and any future VMs such as the k3s nodes. If it fills, writes fail for every VM on it, not only runners.

Runner clones are linked clones of the template's 16 GiB disk. They consume real space only for blocks a job writes (images, build output, the checkout), usually a few GiB, and destroying the clone returns that space. A clone can never exceed its 16 GiB virtual size, so each runner is capped; the risk is clones that are never destroyed.

The controller bounds that independently of its own cleanup logic:

- **Pool cap:** before cloning, it counts VMs in `gha-runners` and refuses to clone when there are already two (one running, one stuck), logging an error for the operator. Worst-case runner usage is therefore about 32 GiB.
- **Stale-VM cleanup:** pool VMs past their startup or job timeout are destroyed during reconciliation.
- **Monitoring:** once the monitoring VM exists, alert on `local-lvm` data usage. This protects every VM on the pool, not only runners.

A dedicated thin pool for runners would isolate their space and narrow `Datastore.AllocateSpace` to it, but linked clones must live on the template's storage, so the template would move as well, and it requires LVM changes on the host. It is not worth it while the pool cap holds usage far below free space.

## Not yet verified

- That `cicustom` and `ciupgrade` changes need only `VM.Config.Cloudinit`. The config endpoint accepts any `VM.Config.*` privilege at entry and checks individual options afterwards. If the controller gets a 403 there, the missing privilege belongs in `GhaRunnerPool`.

Confirmed by the controller's first live run: the clone's `SDN.Use` check is on `/sdn/zones/localnetwork/vmbr0` for a plain Linux bridge (Proxmox names the path in its 403).
