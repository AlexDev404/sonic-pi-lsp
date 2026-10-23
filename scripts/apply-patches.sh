#!/usr/bin/env bash
# Applies patches/*.patch to the external/sonic-pi submodule. A patch that
# is already applied is left alone, so this is safe to run again; one that
# no longer applies (after scripts/update-sonic-pi.sh, say) stops the build
# so it can be refreshed.
#
# Then it installs the synthdefs those patches register, from their own
# submodules. :grandpiano's comes from external/sonicpi-grandpiano.
#
# --ignore-whitespace lets the patches apply to a checkout with CRLF line
# endings (git's core.autocrlf on Windows).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SP="$ROOT/external/sonic-pi"

shopt -s nullglob
for p in "$ROOT"/patches/*.patch; do
  name="patches/$(basename "$p")"
  if git -C "$SP" apply --ignore-whitespace --reverse --check "$p" 2>/dev/null; then
    echo "$name: already applied"
  elif git -C "$SP" apply --ignore-whitespace "$p"; then
    echo "$name: applied"
  else
    echo "$name does not apply to external/sonic-pi ($(git -C "$SP" describe --tags --always)); refresh it" >&2
    exit 1
  fi
done

# The synthdef's decay opt is a stretch factor; Sonic Pi would treat an opt
# called decay as an envelope time (scaling it by the BPM), so it is
# renamed to decay_scale, as SynthGrandPiano in patches/grandpiano.patch
# declares it.
GP="$ROOT/external/sonicpi-grandpiano/grandpiano.scsyndef"
if [ ! -f "$GP" ]; then
  echo "external/sonicpi-grandpiano is empty: run git submodule update --init --recursive" >&2
  exit 1
fi
ruby "$ROOT/scripts/synthdef-rename.rb" "$GP" "$SP/etc/synthdefs/compiled/sonic-pi-grandpiano.scsyndef" \
  sonic-pi-grandpiano decay=decay_scale
echo "etc/synthdefs/compiled/sonic-pi-grandpiano.scsyndef: from external/sonicpi-grandpiano ($(git -C "$ROOT/external/sonicpi-grandpiano" rev-parse --short HEAD))"
