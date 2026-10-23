#!/usr/bin/env bash
# Stages a runnable install tree in DEST:
#
#   DEST/sonicpi, DEST/sonicpid     the CLI and the service (from build/bin)
#   DEST/lint/                      the linter and formatter
#   DEST/sonic-pi/                  the Sonic Pi server, without the GUI,
#                                   its docs or its examples
#
# Run it after scripts/build-server-*.sh and `make build`. Works on Linux
# and in Git Bash on Windows.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SP="$ROOT/external/sonic-pi"
DEST="${1:?usage: stage.sh DEST}"
EXE=""
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) EXE=".exe" ;; esac

rm -rf "$DEST"
mkdir -p "$DEST/sonic-pi/app/server" "$DEST/sonic-pi/app/config" "$DEST/sonic-pi/etc/synthdefs"

for b in sonicpi sonicpid; do
  if [ -f "$ROOT/build/bin/$b$EXE" ]; then
    cp "$ROOT/build/bin/$b$EXE" "$DEST/"
  fi
done

mkdir -p "$DEST/lint"
cp "$ROOT/lint/splint.rb" "$DEST/lint/"
cp -r "$ROOT/lint/lib" "$DEST/lint/"

# The server: Ruby code, native extensions and vendored gems (lib only, as
# Sonic Pi's own release does), and the native binaries.
[ -f "$SP/VERSION" ] && cp "$SP/VERSION" "$DEST/sonic-pi/"
cp -r "$SP/app/config/user-examples" "$DEST/sonic-pi/app/config/"
cp -r "$SP/app/server/ruby" "$DEST/sonic-pi/app/server/"
rm -rf "$DEST/sonic-pi/app/server/ruby/test" "$DEST/sonic-pi/app/server/ruby/Rakefile"
for f in "$DEST"/sonic-pi/app/server/ruby/vendor/*/*; do
  [ "$(basename "$f")" = "lib" ] || rm -rf "$f"
done
cp -r "$SP/app/server/native" "$DEST/sonic-pi/app/server/"
rm -f "$DEST/sonic-pi/app/server/native/.gitkeep" "$DEST/sonic-pi/app/server/native/"*.pdb

# Sonic Pi 4 only: the Tau release.
TAU="$SP/app/server/beam/tau"
if [ -d "$TAU/_build/prod/rel" ]; then
  mkdir -p "$DEST/sonic-pi/app/server/beam/tau/_build/prod"
  cp -r "$TAU/_build/prod/rel" "$DEST/sonic-pi/app/server/beam/tau/_build/prod/"
  cp "$TAU"/boot-*.sh "$TAU"/boot-*.bat "$DEST/sonic-pi/app/server/beam/tau/" 2>/dev/null || true
fi

# What the server loads at run time: samples, synthdefs, buffers (the
# random stream) and snippets. Docs, examples and the tutorial stay out.
for d in samples buffers snippets; do
  cp -r "$SP/etc/$d" "$DEST/sonic-pi/etc/"
done
cp -r "$SP/etc/synthdefs/compiled" "$DEST/sonic-pi/etc/synthdefs/"

echo "Staged $(du -sh "$DEST" | cut -f1) in $DEST"
