#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Builds what the native core links from Sonic Pi's runtime:
#
#   <out>/mruby/<target>/lib/libmruby.a   mruby, for this machine (host) and each Android ABI
#   <out>/mruby/<target>/flags            the compile flags each target's headers need
#   <out>/runtime/runtime_irep.c          the runtime's Ruby as bytecode (sp_runtime_irep)
#
#   native/build-mruby.sh <out> [abi,abi...]
#
# ABIs need ANDROID_NDK_HOME. The runtime's Ruby is concatenated in the order
# sonic-pi/app/web/scripts/build-runtime.sh loads it.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SP="$(cd "$HERE/../sonic-pi" && pwd)"
OUT="$(mkdir -p "$1" && cd "$1" && pwd)"
ABIS="${2:-}"
RUNTIME="$SP/app/web/runtime"
[ -f "$RUNTIME/mruby/Rakefile" ] || { echo "no mruby: in sonic-pi, git submodule update --init --recursive app/web/runtime/mruby" >&2; exit 1; }

export SP_RUNTIME="$RUNTIME" SP_ANDROID_ABIS="$ABIS"
export MRUBY_CONFIG="$HERE/mruby_config.rb" MRUBY_BUILD_DIR="$OUT/mruby"
mkdir -p "$OUT/runtime"
(cd "$RUNTIME/mruby" && ruby -e 'require "rake"; Rake.application.run' -- -j"$(nproc 2>/dev/null || sysctl -n hw.ncpu)" all) > "$OUT/mruby-build.log" 2>&1 \
  || { tail -40 "$OUT/mruby-build.log"; exit 1; }

# Each target's compile flags (defines such as MRB_INT64), as mruby wrote them.
for dir in "$OUT"/mruby/*/; do
  flags="$dir/lib/libmruby.flags.mak"
  [ -f "$flags" ] || continue
  sed -n 's/^MRUBY_CFLAGS = *//p' "$flags" | sed -e "s|\$(MRUBY_PACKAGE_DIR)|${dir%/}|g" -e 's/"//g' > "$dir/flags"
done

LIB="$RUNTIME/lib/sonic_pi"
SERVER_LIB="$SP/app/server/ruby/lib/sonicpi"
cat "$LIB/errors.rb" "$LIB/defaults.rb" "$SERVER_LIB/validation.rb" "$LIB/float_format.rb" "$LIB/ring.rb" \
    "$LIB/rand.rb" "$LIB/rand_verbs.rb" "$LIB/note.rb" "$LIB/time_state.rb" "$LIB/preparser.rb" \
    "$RUNTIME/data/synths.rb" "$RUNTIME/data/theory.rb" "$RUNTIME/data/lang.rb" "$RUNTIME/data/samples.rb" \
    "$LIB/theory.rb" "$LIB/samples.rb" "$LIB/cue_history.rb" "$LIB/scheduler.rb" "$LIB/lang.rb" \
    "$LIB/lang_more.rb" "$LIB/synth_meta.rb" "$LIB/adapter.rb" > "$OUT/runtime/runtime.rb"
"$OUT/mruby/host/bin/mrbc" -B sp_runtime_irep -o "$OUT/runtime/runtime_irep.c" "$OUT/runtime/runtime.rb"
echo "mruby: $(ls -d "$OUT"/mruby/*/lib/libmruby.a | sed "s|$OUT/||" | tr '\n' ' ')"
