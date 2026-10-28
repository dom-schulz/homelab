#!/usr/bin/env bash
# Register a just-in-time runner and print its encoded config.
#
# The template's gha-runner.path starts the runner once this config is written
# to /etc/gha-runner/jitconfig through the guest agent, so run it right before
# writing (the registration exists on GitHub from this point):
#   scripts/generate-runner-jitconfig.sh gha-runner-test-2 \
#     | ssh root@<pve> 'pvesh create /nodes/pve/qemu/<vmid>/agent/file-write \
#         --file /etc/gha-runner/jitconfig --content "$(cat)"'
set -Eeuo pipefail

runner_name="${1:?usage: $0 <runner-name>}"
runner_labels="${RUNNER_LABELS:-homelab-ephemeral}"

repo="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"

label_args=(-f 'labels[]=self-hosted' -f 'labels[]=linux' -f 'labels[]=x64')
IFS=',' read -ra extra_labels <<<"${runner_labels}"
for label in "${extra_labels[@]}"; do
  label_args+=(-f "labels[]=${label}")
done

gh api -X POST "repos/${repo}/actions/runners/generate-jitconfig" \
  -f name="${runner_name}" \
  -F runner_group_id=1 \
  "${label_args[@]}" \
  --jq .encoded_jit_config
