# Notes for coding agents

This repository is a command-line Sonic Pi:

- `cmd/` and `internal/`: the Go CLI (`sonicpi`) and service (`sonicpid`).
- `lint/`: the Ruby linter and formatter.
- `external/sonic-pi`: the Sonic Pi server, as a git submodule.
- `patches/`: the changes this repository makes to that submodule.

Most tasks need no running server, so don't build or boot one unless your
task needs one:

| Task | Needs |
| --- | --- |
| Go code, the CLI's flags and output | `make build`, `go vet ./...`, `go test ./...` (the service tests use a fake server in `internal/service/testdata`) |
| The linter, the formatter, synth/FX/opt knowledge | `ruby -EUTF-8 lint/test/test_splint.rb` and `build/bin/sonicpi lint --use-defaults FILE`. These read the submodule's Ruby sources, not a built server. |
| Hearing that code plays (a new synth, `run`, `record`, the service) | The built server and the container setup below |

## Running the server in a container

The container has no sound card. Sonic Pi 5's engine (SuperSonic) still
starts headless, but then it never answers the Ruby server's boot
handshake. The boot fails after 30 seconds with:

    The SuperSonic audio server did not respond within 30 seconds.
    It was started on port NNNNN but never answered the boot handshake.

Don't debug this, and don't try PipeWire, `snd-aloop` or ALSA settings.
Give the engine a dummy JACK server and tell Sonic Pi to use JACK. These
steps are known to work:

```bash
# 1. Build dependencies (Ubuntu 24.04; the Rust toolchain must be on PATH too)
apt-get update && apt-get install -y --no-install-recommends \
  build-essential cmake ninja-build pkg-config git ruby ruby-dev libssl-dev \
  libaubio-dev libasound2-dev libjack-jackd2-dev libpipewire-0.3-dev libudev-dev \
  libfreetype-dev libcurl4-openssl-dev jackd2

# 2. The submodule, the server and the CLI. The first build takes several
#    minutes; build/external is reused after that.
git submodule update --init --recursive
scripts/build-server-linux.sh        # also applies patches/
make build                           # build/bin/sonicpi, build/bin/sonicpid

# 3. A JACK server with no hardware behind it (keep it running). Wait
#    for it: if the server boots before JACK is up, the engine starts
#    headless and the boot fails as above.
(jackd -d dummy -r 48000 > /tmp/jackd.log 2>&1 &)
jack_wait -w -t 10

# 4. A throwaway Sonic Pi home, so nothing touches ~/.sonic-pi
S=$(mktemp -d)
export SONIC_PI_HOME=$S/home SONICPI_LOG_DIR=$S/logs \
       SONICPI_STATE_DIR=$S/state XDG_RUNTIME_DIR=$S/xdg
mkdir -p $SONIC_PI_HOME $XDG_RUNTIME_DIR
build/bin/sonicpi config set audio-driver JACK

# 5. Boots on first use (about 4 seconds), then plays
build/bin/sonicpi eval 'play 60'
```

Export the four variables in every shell that runs `sonicpi`. The CLI
finds its running service through `SONICPI_STATE_DIR`/`XDG_RUNTIME_DIR`.
It finds the server through `SONIC_PI_HOME`, which also holds the
`audio-driver` setting. Without the variables it starts a second,
JACK-less service, and that one hangs at boot.

### Checking that sound came out

`sonicpi record` captures the engine's output whether or not a device is
there. Start it in the background, play, then read the WAV's peak:

```bash
(build/bin/sonicpi record $S/out.wav --duration 6s > $S/rec.out 2>&1 &)
sleep 1
build/bin/sonicpi eval 'use_synth :grandpiano
play :c4
sleep 1
play :e4'
sleep 6; cat $S/rec.out
python3 - $S/out.wav <<'EOF'
import sys, wave
w = wave.open(sys.argv[1]); d = w.readframes(w.getnframes()); n = w.getsampwidth()
peak = max(abs(int.from_bytes(d[i:i+n], 'little', signed=True)) for i in range(0, len(d), n))
print('peak', peak / 2 ** (8 * n - 1))   # 0 means silence
EOF
```

### When it goes wrong

- **Look at the logs, not the processes.** Read `$SONIC_PI_HOME/.sonic-pi/log/supersonic.log` (the engine) and `daemon.log`, or run `build/bin/sonicpi logs --source daemon`.
  - `device-setup ... failed: no channels` or `headless (no audio device)` means JACK wasn't up when the server booted, or `audio-driver` isn't `JACK`. Check with `jack_lsp`, then `build/bin/sonicpi reboot`.
  - The engine's xrun warnings in `/tmp/jackd.log` and its `[overrun]` lines are normal here. The dummy driver keeps strict time on a shared CPU.
  - `open /dev/snd/seq failed` is MIDI. It's harmless.
- **Stopping and restarting.** `build/bin/sonicpi service stop` stops it, and `build/bin/sonicpi reboot` restarts the server after a config change. `sonicpi status` shows the state.
- **Killing processes.** Don't use `pkill -f sonicpi`: the pattern also matches the shell running the command, which kills your own tool call. Use `pkill -x sonicpid`, or kill the pid that `sonicpi status` shows.
- **Not this repository's job.** Don't install the `.deb` to test. Its Recommends pull in PipeWire, and none of the above needs it.

## Patching Sonic Pi

The submodule is pinned to a Sonic Pi release and isn't edited in place. Its
changes live in `patches/*.patch`, and `scripts/apply-patches.sh` applies
them. Applying is idempotent. `scripts/build-server-linux.sh`,
`scripts/stage.sh` and CI all run it.

`.gitmodules` sets `ignore = dirty`, so a patched submodule doesn't show in
`git status`. To change or add a patch:

```bash
cd external/sonic-pi
# edit files; for new files: git add -N PATH
git diff --binary > ../../patches/NAME.patch
git checkout -- . && git clean -fd   # back to the pinned release
cd ../.. && scripts/apply-patches.sh
```

After `scripts/update-sonic-pi.sh` moves the submodule to a new release, it
checks that the patches still apply. Refresh any patch that doesn't.

### patches/grandpiano.patch

This patch adds `:grandpiano`, an additive piano synth that uses only core
UGens, so it runs on SuperSonic:

- `etc/synthdefs/compiled/sonic-pi-grandpiano.scsyndef`.
- `SynthGrandPiano` in `app/server/ruby/lib/sonicpi/synths/synthinfo.rb`.

The synthdef's `decay` parameter is renamed to `decay_scale`. It is a
stretch factor, and Sonic Pi treats an opt named `decay` as an envelope
time: it scales it by the BPM and subtracts it in `duration:`. Only `amp`
and `pan` are control-rate, so only they are marked modulatable.
