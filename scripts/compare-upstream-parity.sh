#!/usr/bin/env bash
# Compare row counts between upstream PeeringDB and the peeringdb-plus
# mirror for a small probe set, to verify whether our pdbcompat layer
# returns the same result counts upstream does.
#
# Usage:
#   PDB_API_KEY=<your-key> ./scripts/compare-upstream-parity.sh > /tmp/parity-results.txt
#
# Without an API key upstream rate-limits anonymous requests to 1/hour.
# WITH a key, upstream allows ~60 req/min. Either way, this script
# issues at most 8 upstream requests, so even unauthenticated it would
# take ~8 hours — set PDB_API_KEY.
#
# Output: a results file you can paste back to compare row counts +
# response sizes side by side.
#
# Read-only requests only (GETs). Safe to run.

set -u

UPSTREAM="${UPSTREAM_BASE:-https://www.peeringdb.com}"
MIRROR="${MIRROR_BASE:-https://peeringdb-plus.fly.dev}"
KEY="${PDB_API_KEY:-}"

auth_header=()
if [ -n "$KEY" ]; then
  auth_header=(-H "Authorization: Api-Key $KEY")
fi

# probe a small but representative set: small/medium/large entities
# at depth=0 (the contentious case), bare URL + ?limit=0.
PROBES=(
  "campus"     # small (~525 rows)
  "carrier"    # small (~1.9k)
  "ix"         # medium (~9k)
  "org"        # large (~33k active)
)

probe_one() {
  local label="$1" base="$2" path="$3"
  # -w prints body size + http code AFTER the body; we discard the body
  # but keep the row count by piping through jq.
  local tmp
  tmp=$(mktemp)
  local code size_dl
  read -r code size_dl < <(
    curl -sS \
      "${auth_header[@]}" \
      -o "$tmp" \
      -w "%{http_code} %{size_download}\n" \
      "$base$path"
  )
  local rows="?"
  if [ "$code" = "200" ]; then
    # upstream returns {meta, data} — both surfaces should match this shape
    rows=$(python3 -c "
import json, sys
try:
  d = json.load(open(sys.argv[1]))
  print(len(d.get('data', [])) if isinstance(d.get('data'), list) else '?')
except Exception as e:
  print('parse-err:', e)
" "$tmp")
  fi
  printf "  %-9s %-30s %s rows=%s bytes=%s\n" "$label" "$path" "$code" "$rows" "$size_dl"
  rm -f "$tmp"
}

printf "Comparing upstream vs mirror (host: $(hostname); date: $(date -u +%Y-%m-%dT%H:%M:%SZ))\n"
printf "auth: %s\n\n" "$([ -n "$KEY" ] && echo 'with PDB_API_KEY' || echo 'ANONYMOUS — upstream rate-limited')"

for ty in "${PROBES[@]}"; do
  printf "==== /api/%s ====\n" "$ty"
  probe_one "upstream" "$UPSTREAM" "/api/$ty"
  probe_one "mirror"   "$MIRROR"   "/api/$ty"
  printf "\n"
  probe_one "upstream" "$UPSTREAM" "/api/$ty?limit=0"
  probe_one "mirror"   "$MIRROR"   "/api/$ty?limit=0"
  printf "\n"
done

# Single-id probe to ensure detail endpoints behave the same.
printf "==== /api/net/15169 (single-id) ====\n"
probe_one "upstream" "$UPSTREAM" "/api/net/15169"
probe_one "mirror"   "$MIRROR"   "/api/net/15169"

# Deep query to confirm depth handling matches.
printf "\n==== /api/org?depth=2 (depth handling) ====\n"
probe_one "upstream" "$UPSTREAM" "/api/org?depth=2"
probe_one "mirror"   "$MIRROR"   "/api/org?depth=2"

printf "\nDone. Compare row counts column-wise: any divergence is a parity bug.\n"
printf "Expected: rows match between upstream and mirror for every probe row.\n"
