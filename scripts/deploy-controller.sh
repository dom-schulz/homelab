#!/usr/bin/env bash
# Build the controller and install it on the controller container over SSH.
# The first run also creates the service user and config directory; secrets
# are copied separately (terraform/controller/README.md).
#
# Usage: scripts/deploy-controller.sh root@<controller address>
#
# Restarting the service stops an in-flight job: the controller destroys its VM
# and the job fails.
set -euo pipefail

target=${1:?usage: $0 root@<controller address>}
repo=$(cd "$(dirname "$0")/.." && pwd)
build=$(mktemp -d)
trap 'rm -rf "$build"' EXIT

(cd "$repo/controller" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$build/gha-controller" .)

stage=$(ssh "$target" mktemp -d)
scp -q "$build/gha-controller" "$repo/controller/deploy/gha-controller.service" "$target:$stage/"

ssh "$target" STAGE="$stage" bash -s <<'REMOTE'
set -euo pipefail
id gha-controller >/dev/null 2>&1 ||
  useradd --system --home-dir /var/lib/gha-controller --no-create-home --shell /usr/sbin/nologin gha-controller
install -d -m 0750 -o root -g gha-controller /etc/gha-controller
install -m 0755 "$STAGE/gha-controller" /usr/local/bin/gha-controller
install -m 0644 "$STAGE/gha-controller.service" /etc/systemd/system/gha-controller.service
rm -rf "$STAGE"
systemctl daemon-reload
systemctl enable --quiet gha-controller.service

if [ -f /etc/gha-controller/controller.env ]; then
  systemctl restart gha-controller.service
  systemctl --no-pager --lines=5 status gha-controller.service
else
  echo "Installed. Copy /etc/gha-controller/controller.env and the App key, then: systemctl start gha-controller"
fi
REMOTE
