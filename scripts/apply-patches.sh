#!/usr/bin/env bash
# Applies patches/*.patch to the external/sonic-pi submodule. A patch that
# is already applied is left alone, so this is safe to run again; one that
# no longer applies (after scripts/update-sonic-pi.sh, say) stops the build
# so it can be refreshed.
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
