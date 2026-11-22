#!/usr/bin/env bash
# Make pve's root authorized_keys allow exactly the keys in admin-keys.pub.
# They go in a marked block; lines outside it, such as pve's own root@pve key,
# are kept, minus any copies of keys the block now holds. Shows the change and
# asks before writing; the old file is kept as authorized_keys.bak.
#
# Usage: scripts/sync-pve-admin-keys.sh root@<pve address>
set -euo pipefail

target=${1:?usage: $0 root@<pve address>}
repo=$(cd "$(dirname "$0")/.." && pwd)
keys_file="$repo/admin-keys.pub"
begin='# BEGIN homelab admin keys (scripts/sync-pve-admin-keys.sh)'
end='# END homelab admin keys'

keys=$(grep -Ev '^[[:space:]]*(#|$)' "$keys_file") || { echo "no keys in $keys_file" >&2; exit 1; }

# Refuse a list that would lock this machine out.
own=false
for pub in ~/.ssh/*.pub; do
  [ -e "$pub" ] && grep -qF "$(awk '{print $2}' "$pub")" <<<"$keys" && own=true
done
$own || { echo "none of this machine's ~/.ssh/*.pub keys is in $keys_file; refusing to lock you out" >&2; exit 1; }

current=$(ssh "$target" cat /root/.ssh/authorized_keys)
[ -n "$current" ] || { echo "$target returned an empty authorized_keys; refusing to guess what to keep" >&2; exit 1; }
kept=$(awk -v b="$begin" -v e="$end" -v blobs="$(awk '{print $2}' <<<"$keys")" '
  BEGIN { n = split(blobs, a, "\n"); for (i = 1; i <= n; i++) managed[a[i]] = 1 }
  $0 == b { skip = 1; next }
  $0 == e { skip = 0; next }
  skip { next }
  { for (i = 1; i <= NF; i++) if ($i in managed) next; print }
' <<<"$current")
new=$(printf '%s\n%s\n%s\n%s\n' "$kept" "$begin" "$keys" "$end" | sed '/./,$!d')

if [ "$new" = "$current" ]; then
  echo "pve already allows exactly these keys."
  exit 0
fi
diff -u --label "pve (now)" --label "pve (after)" <(echo "$current") <(echo "$new") || true
echo
echo "Keys outside the managed block stay; remove any you no longer trust by hand."
read -rp "Write this to $target? [y/N] " answer
[ "$answer" = y ] || { echo "Nothing written."; exit 1; }

# On pve, /root/.ssh/authorized_keys links into the cluster filesystem.
ssh "$target" 'f=$(readlink -f /root/.ssh/authorized_keys) && cp "$f" "$f.bak" && cat > "$f"' <<<"$new"
echo "Written; the previous file is $target:/root/.ssh/authorized_keys.bak."
