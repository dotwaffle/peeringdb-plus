#!/usr/bin/env bash
# Compare the JSON shape of single-record responses between upstream
# PeeringDB and the peeringdb-plus mirror, with the SAME auth tier on
# both sides — eliminates Phase 64 POC-privacy-filter as a confound.
#
# Usage:
#   ./scripts/compare-upstream-fields.sh > /tmp/field-diff.txt
#
# Optional: PDB_API_KEY=<key> if you want to compare authenticated-tier
# responses. Without the key, both sides are queried as ANONYMOUS —
# which is what the mirror serves to public callers anyway, so the
# anonymous-vs-anonymous comparison is the most accurate test of
# parity between upstream and the mirror's public-facing surface.
#
# Upstream's anonymous rate limit is 100/second (mainsite/settings.py
# API_THROTTLE_RATE_ANON), with a 10/minute soft cap on responses
# larger than 1 MB. The 6-7 probes this script issues are well below
# either limit.
#
# Three probe records picked to cover the field surface:
#   /api/net/15169         — Google, dense POC + ixlan relations
#   /api/org/1989          — ARIN, classic test org
#   /api/ix/26             — DE-CIX, dense IX with many fac/ixlan refs
#
# Output for each: top-level keys present in upstream but not mirror,
# vice versa, and which top-level keys differ in size (sub-arrays of
# different length, suggesting tier-filtered data).

set -u

UPSTREAM="${UPSTREAM_BASE:-https://www.peeringdb.com}"
MIRROR="${MIRROR_BASE:-https://peeringdb-plus.fly.dev}"
KEY="${PDB_API_KEY:-}"

# Auth header is empty when PDB_API_KEY is unset — both sides queried
# anonymously, which is the apples-to-apples comparison of public
# surfaces. With a key, both sides see authenticated-tier responses.
auth=()
auth_label="anonymous (both sides)"
if [ -n "$KEY" ]; then
  auth=(-H "Authorization: Api-Key $KEY")
  auth_label="authenticated via PDB_API_KEY (both sides)"
fi

probe() {
  local label="$1" path="$2"
  local up_file mr_file
  up_file=$(mktemp)
  mr_file=$(mktemp)
  curl -sS "${auth[@]}" -o "$up_file" "$UPSTREAM$path"
  curl -sS "${auth[@]}" -o "$mr_file" "$MIRROR$path"

  local up_size mr_size
  up_size=$(wc -c < "$up_file")
  mr_size=$(wc -c < "$mr_file")
  printf "==== %s : %s ====\n" "$label" "$path"
  printf "  upstream  bytes=%s\n" "$up_size"
  printf "  mirror    bytes=%s\n" "$mr_size"
  printf "  delta     %+d bytes\n\n" $((mr_size - up_size))

  # Top-level keys diff (assumes single-record envelope: {meta, data:[obj]})
  printf "  -- top-level field set diff (data[0]) --\n"
  diff <(jq -r '.data[0] | keys[]' "$up_file" | sort) \
       <(jq -r '.data[0] | keys[]' "$mr_file" | sort) \
    | grep -E '^[<>]' \
    | sed -e 's/^</  ONLY-UPSTREAM: /' -e 's/^>/  ONLY-MIRROR:   /' \
    || printf "  (top-level keys identical)\n"

  # For each top-level key whose value is a list, compare lengths.
  # That's the canonical signal for POC / public-only filtering.
  printf "\n  -- list-valued field length comparison --\n"
  jq -r '.data[0] | to_entries[] | select(.value | type == "array") | .key' "$up_file" \
    | sort | while read -r k; do
        up_len=$(jq ".data[0].\"$k\" | length" "$up_file")
        mr_len=$(jq ".data[0].\"$k\" | length" "$mr_file" 2>/dev/null || echo MISSING)
        marker=""
        if [ "$up_len" != "$mr_len" ]; then
          marker="  ← DIFFERS"
        fi
        printf "  %-32s up=%s  mr=%s%s\n" "$k" "$up_len" "$mr_len" "$marker"
      done

  printf "\n  -- spot-check POC visibility (likely cause of size delta) --\n"
  # If poc_set is present in upstream and shorter on mirror, dump the
  # visibility tier of each upstream poc to show which got filtered.
  if jq -e '.data[0].poc_set' "$up_file" > /dev/null 2>&1; then
    printf "  upstream poc_set visibility breakdown:\n"
    jq -r '.data[0].poc_set | group_by(.visible) | map({visible: .[0].visible, count: length}) | .[] | "    \(.visible): \(.count)"' "$up_file"
    if jq -e '.data[0].poc_set' "$mr_file" > /dev/null 2>&1; then
      printf "  mirror poc_set visibility breakdown:\n"
      jq -r '.data[0].poc_set | group_by(.visible) | map({visible: .[0].visible, count: length}) | .[] | "    \(.visible): \(.count)"' "$mr_file"
    fi
  fi

  printf "\n"
  rm -f "$up_file" "$mr_file"
}

printf "Field-shape comparison (host: $(hostname); date: $(date -u +%Y-%m-%dT%H:%M:%SZ))\n"
printf "Auth tier: %s\n\n" "$auth_label"

probe "google-net"      "/api/net/15169?depth=2"
probe "arin-org"        "/api/org/1989?depth=2"
probe "decix-ix"        "/api/ix/26?depth=2"

printf "Done.\n\n"
printf "Interpretation:\n"
printf "  - If sizes match (delta near zero): mirror's anonymous tier\n"
printf "    correctly filters the same fields upstream's anonymous tier\n"
printf "    filters — Phase 64/58 privacy enforcement is parity-correct.\n"
printf "  - If sizes still differ: there is a real parity gap independent\n"
printf "    of POC visibility filtering — investigate the field diff +\n"
printf "    list-length section above to identify the missing data.\n"
