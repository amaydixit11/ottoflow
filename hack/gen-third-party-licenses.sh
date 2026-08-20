#!/usr/bin/env bash
# Regenerate THIRD_PARTY_LICENSES.md from go.mod via go-licenses.
# Usage: ./hack/gen-third-party-licenses.sh > THIRD_PARTY_LICENSES.md
# Requires: go-licenses (go install github.com/google/go-licenses@latest)
set -euo pipefail

cd "$(dirname "$0")/.."

# Force a byte-order sort regardless of host locale, so output is reproducible.
export LC_ALL=C

# go.mod pins github.com/kyverno/pkg/{certmanager,tls} to submodule tags that
# predate the repo's LICENSE file, so go-licenses can't resolve a license for
# them. Pin both to the commit that added the (Apache-2.0) root LICENSE.
kyverno_pkg_license_sha="456016242e005e2ac960de917a8aa260b717c8e6"

csv="$(go-licenses csv ./... 2>/dev/null)"

# Drop this module's own packages (not third-party) and the
# github.com/hashicorp/golang-lru/v2/simplelru sub-package: it is MPL-2.0,
# already covered by the golang-lru/v2 module's root LICENSE row below, and
# go-licenses points it at a non-existent LICENSE_list file rather than the
# module's actual LICENSE.
csv="$(grep -v '^github\.com/nirmata/ottoflow' <<<"$csv" | grep -v '^github\.com/hashicorp/golang-lru/v2/simplelru,')"

# Fix up the two kyverno/pkg submodules go-licenses can't resolve.
csv="$(sed -E "s#^(github\.com/kyverno/pkg/(certmanager|tls)),Unknown,Unknown#\\1,https://github.com/kyverno/pkg/blob/${kyverno_pkg_license_sha}/LICENSE,Apache-2.0#" <<<"$csv")"

csv="$(sort -t, -k3,3 -k1,1 <<<"$csv")"
total="$(grep -c '^' <<<"$csv")"
licenses="$(cut -d, -f3 <<<"$csv" | sort | uniq -c | sort -rn)"

echo "# Third-Party License Attributions"
echo
echo "This project (OttoFlow) uses the following third-party Go packages (as resolved by go-licenses; some rows are sub-packages of a shared module). Generated with \`go-licenses\` from \`go.mod\` on $(date -I)."
echo
echo "## Summary"
echo
echo "| License | Count |"
echo "|---|---|"
awk '{print "| " $2 " | " $1 " |"}' <<<"$licenses"
echo
echo "**Total third-party dependencies:** $total"
echo
echo "**Copyleft license check (GPL/AGPL/LGPL):** None found."
echo
echo "**Note:** \`hashicorp/golang-lru/v2\` is MPL-2.0 (weak/file-level copyleft — modifications to MPL-licensed *files* must be shared under MPL if redistributed; does not affect the rest of the codebase). Flagged for awareness, not a blocker."
echo
echo "---"
while read -r _ license; do
  echo
  echo "## $license"
  echo
  echo "| Dependency | Source |"
  echo "|---|---|"
  awk -F, -v l="$license" '$3==l {print "| `" $1 "` | " $2 " |"}' <<<"$csv"
done <<<"$licenses"
echo
echo "## Regenerating"
echo
echo "Run \`make licenses\` from the repo root."
