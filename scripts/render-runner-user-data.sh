#!/usr/bin/env bash
# Print cloud-init user data for one ephemeral runner clone.
#
# This registers a just-in-time runner with GitHub, so run it right before
# starting the clone:
#   scripts/render-runner-user-data.sh gha-runner-test-1 > gha-runner-test-1.yaml
#
# Clones get no SSH key by default; debug with qm guest exec instead, or opt
# in with SSH_PUBLIC_KEY_FILE=~/.ssh/id_ed25519.pub.
set -Eeuo pipefail

runner_name="${1:?usage: $0 <runner-name>}"
runner_user="${RUNNER_USER:-ubuntu}"
runner_labels="${RUNNER_LABELS:-homelab-ephemeral}"
runner_version="2.338.1"
runner_sha256="08e9ccb3604d2462c24b5e33604dc30810296ca15cc65f23cffd4317eb144c37"
ssh_public_key_file="${SSH_PUBLIC_KEY_FILE:-}"

repo="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
ssh_keys_yaml=""
if [[ -n "${ssh_public_key_file}" ]]; then
  ssh_keys_yaml="ssh_authorized_keys:
  - '$(<"${ssh_public_key_file}")'"
fi

label_args=(-f 'labels[]=self-hosted' -f 'labels[]=linux' -f 'labels[]=x64')
IFS=',' read -ra extra_labels <<<"${runner_labels}"
for label in "${extra_labels[@]}"; do
  label_args+=(-f "labels[]=${label}")
done

jit_config="$(gh api -X POST "repos/${repo}/actions/runners/generate-jitconfig" \
  -f name="${runner_name}" \
  -F runner_group_id=1 \
  "${label_args[@]}" \
  --jq .encoded_jit_config)"

cat <<EOF
#cloud-config
hostname: ${runner_name}
manage_etc_hosts: true
${ssh_keys_yaml}
write_files:
  - path: /etc/gha-runner/jitconfig
    owner: ${runner_user}:${runner_user}
    permissions: '0600'
    defer: true
    content: ${jit_config}
  - path: /usr/local/sbin/gha-runner-start
    permissions: '0755'
    content: |
      #!/usr/bin/env bash
      set -Eeuo pipefail
      exec /opt/actions-runner/run.sh --jitconfig "\$(cat /etc/gha-runner/jitconfig)"
  - path: /etc/systemd/system/gha-runner.service
    permissions: '0644'
    content: |
      [Unit]
      Description=Ephemeral GitHub Actions runner
      Wants=network-online.target
      After=network-online.target docker.service

      [Service]
      User=${runner_user}
      WorkingDirectory=/opt/actions-runner
      ExecStart=/usr/local/sbin/gha-runner-start

      [Install]
      WantedBy=multi-user.target
runcmd:
  - - bash
    - -c
    - |
      set -Eeuo pipefail
      cd /opt/actions-runner
      curl -fsSL -o runner.tar.gz https://github.com/actions/runner/releases/download/v${runner_version}/actions-runner-linux-x64-${runner_version}.tar.gz
      echo '${runner_sha256}  runner.tar.gz' | sha256sum -c -
      tar xzf runner.tar.gz
      rm runner.tar.gz
      ./bin/installdependencies.sh
      chown -R ${runner_user}:${runner_user} /opt/actions-runner
      systemctl daemon-reload
      systemctl start gha-runner.service
EOF
