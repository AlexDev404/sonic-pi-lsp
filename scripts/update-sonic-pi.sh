#!/usr/bin/env bash
# Moves the external/sonic-pi submodule to a Sonic Pi release.
#
#   scripts/update-sonic-pi.sh           the newest release tag (vX.Y.Z)
#   scripts/update-sonic-pi.sh v5.0.0    a given tag
#
# Then build, test and commit the new submodule pointer:
#   scripts/build-server-linux.sh && make test
#   git commit -am "Sonic Pi vX.Y.Z"
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SP="$ROOT/external/sonic-pi"
URL="$(git -C "$ROOT" config -f .gitmodules submodule.external/sonic-pi.url)"

TAG="${1:-}"
if [ -z "$TAG" ]; then
  TAG="$(git ls-remote --tags --refs "$URL" 'v*' | awk -F/ '{print $3}' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -1)"
fi
echo "Sonic Pi $TAG"

git -C "$SP" fetch --depth 1 origin tag "$TAG"
git -C "$SP" checkout -q "$TAG"
git -C "$SP" submodule update --init --recursive --depth 1
git -C "$ROOT" add external/sonic-pi
echo "external/sonic-pi is now at $(git -C "$SP" describe --tags). Build and test, then commit."
