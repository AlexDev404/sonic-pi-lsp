#!/usr/bin/env bash
# Builds the parts of Sonic Pi the CLI runs (everything but the Qt GUI)
# from the external/sonic-pi submodule:
#
#   - the Ruby server's native extensions (rugged, concurrent-ruby)
#   - app/external: aubio_onset and, from Sonic Pi 5, the SuperSonic audio
#     engine (C++ and Rust)
#   - Sonic Pi 4 only: the Tau release (Erlang/Elixir) and its NIFs
#
# Needs: ruby + ruby-dev, cmake, ninja, a C/C++ compiler, libssl-dev,
# libaubio-dev, libasound2-dev, libjack-jackd2-dev, libudev-dev and Rust
# (Sonic Pi 5), or Elixir/Erlang and librtmidi-dev (Sonic Pi 4).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SP="$ROOT/external/sonic-pi"
APP="$SP/app"
BUILD="${BUILD_DIR:-$ROOT/build}"
JOBS="${JOBS:-$(nproc 2>/dev/null || echo 2)}"

if [ ! -f "$APP/server/ruby/bin/daemon.rb" ]; then
  echo "external/sonic-pi is empty: run git submodule update --init --recursive" >&2
  exit 1
fi
if grep -q 'app/external/supersonic' "$SP/.gitmodules" 2>/dev/null && [ ! -f "$APP/external/supersonic/CMakeLists.txt" ]; then
  echo "app/external/supersonic is empty: run git submodule update --init --recursive" >&2
  exit 1
fi

echo "==> patches/"
"$ROOT/scripts/apply-patches.sh"

echo "==> Ruby native extensions (Ruby $(ruby -e 'print RUBY_VERSION'))"
(cd "$APP" && ruby server/ruby/bin/compile-extensions.rb)

echo "==> app/external (aubio_onset, SuperSonic or Tau's NIFs)"
GEN=()
command -v ninja >/dev/null && GEN=(-G Ninja)
cmake "${GEN[@]}" -S "$APP/external" -B "$BUILD/external" -DCMAKE_BUILD_TYPE=Release
cmake --build "$BUILD/external" --config Release -j "$JOBS"

if [ -d "$APP/server/beam/tau" ]; then
  echo "==> Tau release (Sonic Pi 4)"
  (
    cd "$APP/server/beam/tau"
    export MIX_ENV=prod
    mix local.hex --force
    mix local.rebar --force
    mix tau.release
  )
fi

echo "==> Done. The server is ready to run from $SP"
