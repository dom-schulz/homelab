#!/usr/bin/env bash
# Source from the repository root or any subdirectory:
#   source scripts/load-env.sh

repo_root="$(git rev-parse --show-toplevel)" || return 1
env_file="${repo_root}/.env"

if [[ ! -f "${env_file}" ]]; then
  echo "Environment file not found: ${env_file}" >&2
  return 1
fi

set -a
source "${env_file}"
set +a

if [[ -z "${PROXMOX_VE_API_TOKEN:-}" ]]; then
  echo "PROXMOX_VE_API_TOKEN is missing from ${env_file}." >&2
  return 1
fi
