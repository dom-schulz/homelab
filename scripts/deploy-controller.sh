#!/usr/bin/env bash
# Build the controller and install it in its container through pve (pct push
# and pct exec), so only SSH to pve is needed, not the container's address.
#
# Safe to rerun. On a fresh container it also does the one-time setup: the
# service user, /etc/gha-controller, pve's CA certificate, the "pve" hosts
# entry, and the two credentials from ~/.config/homelab (controller.env and
# github-app.pem). Credentials already in the container are never overwritten.
#
# Usage: scripts/deploy-controller.sh root@<pve address> [container id, default 920]
#
# Restarting the service stops an in-flight job: the controller destroys its VM
# and the job fails.
set -euo pipefail

pve=${1:?usage: $0 root@<pve address> [container id]}
vmid=${2:-920}
repo=$(cd "$(dirname "$0")/.." && pwd)
creds=${CONTROLLER_CREDENTIALS_DIR:-$HOME/.config/homelab}
build=$(mktemp -d)
trap 'rm -rf "$build"' EXIT

(cd "$repo/controller" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$build/gha-controller" .)
cp "$repo/controller/deploy/gha-controller.service" "$build/"
for f in controller.env github-app.pem; do
  if [ -f "$creds/$f" ]; then cp "$creds/$f" "$build/"; fi
done

# The stage directory on pve is 0700 and removed when the remote script exits.
stage=$(ssh "$pve" mktemp -d)
scp -q "$build"/* "$pve:$stage/"

ssh "$pve" STAGE="$stage" VMID="$vmid" bash -s <<'REMOTE'
set -euo pipefail
trap 'rm -rf "$STAGE"' EXIT
ct() { pct exec "$VMID" -- "$@"; }

ct sh -c 'id gha-controller >/dev/null 2>&1 ||
  useradd --system --home-dir /var/lib/gha-controller --no-create-home --shell /usr/sbin/nologin gha-controller'
ct install -d -m 0750 -o root -g gha-controller /etc/gha-controller

# The controller verifies pve's certificate. It lists DNS:pve, so the container
# resolves "pve" to pve's address on the API bridge (vmbr1).
pct push "$VMID" /etc/pve/pve-root-ca.pem /usr/local/share/ca-certificates/pve-root-ca.crt --perms 0644
ct update-ca-certificates >/dev/null
api_ip=$(ip -4 -br addr show vmbr1 | awk '{print $3}' | cut -d/ -f1)
ct sh -c "grep -q ' pve\$' /etc/hosts || echo '$api_ip pve' >> /etc/hosts"

pct push "$VMID" "$STAGE/gha-controller" /usr/local/bin/gha-controller --perms 0755
pct push "$VMID" "$STAGE/gha-controller.service" /etc/systemd/system/gha-controller.service --perms 0644
ct systemctl daemon-reload
ct systemctl enable --quiet gha-controller.service

for f in controller.env github-app.pem; do
  if [ -f "$STAGE/$f" ] && ! ct test -f "/etc/gha-controller/$f"; then
    pct push "$VMID" "$STAGE/$f" "/etc/gha-controller/$f" --perms 0600
    ct chown gha-controller:gha-controller "/etc/gha-controller/$f"
    echo "Installed credential $f."
  fi
done

if ct test -f /etc/gha-controller/controller.env -a -f /etc/gha-controller/github-app.pem; then
  ct systemctl restart gha-controller.service
  sleep 3
  ct systemctl --no-pager --lines=5 status gha-controller.service
else
  echo "Installed, but credentials are missing: put controller.env and github-app.pem in ~/.config/homelab and rerun." >&2
  exit 1
fi
REMOTE
