# sonicpi: Sonic Pi from the command line

`sonicpi` is a full command-line client for [Sonic Pi](https://sonic-pi.net)
on Linux and Windows. Write code in any editor, then:

```sh
sonicpi song.rb              # play it, and show the log as it plays
sonicpi lint song.rb         # check it for errors without playing it
sonicpi --watch song.rb      # play it again every time you save it
sonicpi --watch logs         # follow the log
```

The desktop app is not needed. `sonicpi` runs the real Sonic Pi server (the
same language, synths, samples and audio engine as the desktop) in a
background service, and talks to it the way the desktop GUI does. It reads
and writes the desktop's own settings in `~/.sonic-pi`, so the two agree.

Sonic Pi is the [`external/sonic-pi`](external) submodule, pinned to a
release tag (now **v5.0.0**). Moving to a new release is one script; see
[Updating Sonic Pi](#updating-sonic-pi). The builds apply
[`patches/`](patches) to it. For now that is one patch, which adds a
built-in `:grandpiano` synth: an additive grand piano measured from a
sampled Yamaha C5. The synth itself is the
[`external/sonicpi-grandpiano`](https://github.com/AlexDev404/sonicpi-grandpiano)
submodule.

## Contents

- [Install](#install)
- [Playing code](#playing-code)
- [Logs](#logs)
- [Linting](#linting)
- [Formatting](#formatting)
- [Settings](#settings)
- [Everything else](#everything-else)
- [The background service](#the-background-service)
- [How it works](#how-it-works)
- [Building](#building)
- [Releases](#releases)
- [Updating Sonic Pi](#updating-sonic-pi)

## Install

Download a package from the [releases](https://github.com/AlexDev404/sonic-pi-lsp/releases).

**Debian and Ubuntu**: install the `.deb` for your release:

```sh
sudo apt install ./sonicpi-cli_<version>_ubuntu-24.04_amd64.deb
```

The package installs `sonicpi` and `sonicpid`, enables the service as a
systemd user unit for every user, and creates `/var/log/sonic-pi`. Sound goes
through your desktop session's PipeWire, PulseAudio or JACK.

**Other Linux**: unpack the `.tar.gz` and run `./install.sh`. That installs
into `~/.local`, or `/opt` with `sudo`, and sets up the user unit.

**Windows**: run `sonicpi-cli_<version>_windows_x64_installer.exe`. It
installs into Program Files and adds `sonicpi` to `PATH`. It also asks, on
its components page, whether to install the **background service**: a
Windows service that starts with Windows. You can untick it; `sonicpi` then
starts Sonic Pi itself when you first play something. A portable `.zip` is
also published.

Then run:

```sh
sonicpi doctor      # checks the installation
sonicpi song.rb
```

## Playing code

```sh
sonicpi song.rb                 # same as: sonicpi run song.rb
sonicpi run - < song.rb         # from stdin
sonicpi eval 'play 60'          # code on the command line
sonicpi repl                    # type code, hear it at once
sonicpi stop                    # stop everything
```

`sonicpi FILE` plays the file and shows its log in the desktop's format.
It returns when every run has finished. With `live_loop`s that is never,
so Ctrl-C stops the music and returns. Runtime and syntax errors are shown
with the lines of the file around them, and make the command exit with 1.

| Flag | What it does |
| --- | --- |
| `-w`, `--watch` | play the file again each time it is saved: live coding from any editor |
| `-d`, `--detach` | send the code and return at once; it keeps playing |
| `--timeout 30s` | stop after this long |
| `--record out.wav` | record what plays to a WAV file |
| `--lint` | lint first, and do not play if there are errors |
| `--no-stop` | leave the music playing on Ctrl-C |
| `--synths`, `--no-synths` | log synth triggers (default: the `log-synths` setting) |
| `--cues`, `--no-cues` | log cues (default: the `log-cues` setting) |
| `--check-args`, `--external-synths`, `--timing-guarantees`, `--midi-channel N` | override those settings for one run |
| `--timestamps`, `--json`, `-q` | how the log is printed |

Like the desktop, `sonicpi` puts your preferences in front of the code
(`use_debug false` when synth logging is off, `use_arg_checks true`, and
so on), and Sonic Pi's line numbers still match your file.

## Logs

The service writes everything the server reports to a session log:

- **Linux**: `/var/log/sonic-pi/<user>/`. If that cannot be written, it
  uses `~/.local/state/sonic-pi-cli/log/`.
- **Windows**: `%TEMP%\sonic-pi\`.

The directory holds `sonicpi.log` (plain text, one tagged line per log
line) and `events.jsonl` (the same events as JSON). Both are rotated at
16 MB.

```sh
sonicpi logs                    # the last 100 events
sonicpi logs -f                 # follow (same as: sonicpi --watch logs)
sonicpi logs --synths --cues    # include synth triggers and cues
sonicpi logs --no-synths        # just puts, errors and notices
sonicpi logs --errors           # errors only
sonicpi logs --run 12           # one run
sonicpi logs --thread drums     # one live_loop
sonicpi logs --since 10m -n -1  # everything from the last ten minutes
sonicpi logs --source scsynth   # a raw server log: spider, daemon, scsynth, ...
sonicpi logs --json             # machine readable
sonicpi logs --path             # where the log is
sonicpi logs --clear            # start a fresh log
```

`--synths`, `--cues`, `--puts`, `--info` and `--warnings` each have a
`--no-` form. Synths and cues default to the desktop's "log synths" and
"log cues" settings.

## Linting

`sonicpi lint` checks code without playing it. It reads its knowledge from
the bundled Sonic Pi: synth and FX names, every synth's and FX's opts with
their valid ranges, the built-in samples, notes, chords, scales and the
documented API. So the linter always matches the version it ships with.

```sh
sonicpi lint --init             # write a .splintrc (required) listing every rule
sonicpi lint                    # lint every .rb file under the current directory
sonicpi lint song.rb beats/     # files and directories
sonicpi lint --fix song.rb      # fix whitespace, tabs and indentation in place
sonicpi lint --list-rules       # all rules and their defaults
sonicpi lint --explain unknown-opt
```

```
song.rb
  1:12     error   unknown synth :prophett (did you mean :prophet?)  unknown-synth
      1 | use_synth :prophett
        |            ^
  3:10     warning cuttoff: is not an opt of synth :tb303, so it does nothing (did you mean :cutoff?)  unknown-opt
      3 | play 60, cuttoff: 80, release: -1
        |          ^
  3:23     error   release: -1 is invalid for synth :tb303: it must be zero or greater  invalid-opt-value
  10:1     error   live_loop :drums never sleeps or syncs: add a sleep (e.g. sleep 1) or a sync  live-loop-no-sleep

✖ 4 problems (3 errors, 1 warning, 0 info)
```

There are 33 rules. [docs/lint-rules.md](docs/lint-rules.md) describes each
one. They cover:

- **Correctness**: unknown synths, FX, samples, notes, chords and scales;
  opts a synth or FX does not take; opt values outside their range;
  `live_loop`s and `loop`s that never sleep; negative sleeps; a zero BPM.
- **Suspicious code**: undefined functions, duplicate `live_loop` names,
  code after an endless `loop`, `define`s that shadow Sonic Pi's own
  functions, `sync` on a cue nothing sends, missing sample files, mp3s.
- **Performance**: `with_fx` inside a `live_loop`, nested `live_loop`s,
  `in_thread` in a loop.
- **Compatibility**: functions that need the desktop editor, and deprecated
  ones.
- **Style** (off or info by default): line length, trailing whitespace,
  tabs, the final newline, indentation that differs from Sonic Pi's
  auto-align, nesting depth, file length, comment spacing.

### .splintrc

`sonicpi lint` needs a `.splintrc`. It uses the nearest one at or above each
file; `--config FILE` names one, and `--use-defaults` runs without one.
It is YAML (JSON also works):

```yaml
extends: recommended        # recommended, strict, all or none
fail_on: error              # exit 1 at this level or worse: error, warning, info, never
max_warnings: -1            # also fail above this many warnings
format: pretty              # pretty, compact, json, github, sarif, checkstyle
include: ["**/*.rb"]
exclude: ["vendor/**"]
globals: [my_helper]        # functions defined elsewhere (run_file, another buffer)
use_init: true              # count functions from ~/.sonic-pi/config/init.rb as defined
inline_config: true
report_unused_disable: false

rules:
  unknown-opt: error
  line-length: { severity: warning, max: 100 }
  amp-too-high: { max: 3 }
  trailing-whitespace: off

overrides:
  - files: ["sketches/**"]
    rules:
      undefined-function: off
```

On the command line, `--rule ID[=SEVERITY][,OPT=VAL]`, `--disable ID`,
`--only ID`, `--preset NAME`, `--fail-on LEVEL`, `--max-warnings N`,
`--format NAME` and `--quiet` (errors only) override the file. `--stdin`
lints standard input, `--watch` lints again on every change, and
`--print-config FILE` shows the settings in force for a file. The
`github` format annotates pull requests; `sarif` is for code scanning.

In code:

```ruby
play 60, cuttoff: 80   # splint:disable-line unknown-opt
# splint:disable-next-line undefined-function
my_function_from_another_buffer
# splint:disable unknown-sample, sample-path-missing
...
# splint:enable
```

Exit status: 0 when clean, 1 when problems reach `fail_on` (or exceed
`max_warnings`), and 2 for usage or configuration errors, such as a
missing `.splintrc`.

## Formatting

```sh
sonicpi fmt song.rb          # print it auto-aligned
sonicpi fmt -w song.rb       # rewrite it
sonicpi fmt --check *.rb     # exit 1 if any file is not aligned (for CI)
sonicpi fmt --diff song.rb
```

This is the desktop's auto-align (what Run and Alt-M do), using the same
beautifier from the bundled Sonic Pi.

## Settings

`sonicpi config` reads and writes the desktop's own files in
`~/.sonic-pi/config`: `v5-gui-settings.ini` and `v5-audio-settings.toml` for
Sonic Pi 5 (`gui-settings.ini` and `audio-settings.toml` for 4.x). Other
lines in those files are left as they are.

```sh
sonicpi config list                         # settings that matter to the CLI
sonicpi config list --all                   # include audio and desktop-only ones
sonicpi config get log-synths
sonicpi config set log-cues true            # applied to the next run
sonicpi config set midi-enable false        # applied to the running server at once
sonicpi config set audio-output-device "USB Audio"   # needs: sonicpi reboot
sonicpi config set audio.sound_card_sample_rate 48000  # any key of the audio file
sonicpi config set gui.prefs/theme dark     # any key of the GUI file
sonicpi config unset log-cues               # back to the default
sonicpi config edit audio                   # open a file in $EDITOR: gui, audio, init, theme, shortcuts
sonicpi config path init                    # where a file is
```

`config list` says when each setting takes effect: on the next run, live
(the service applies it to the running server), or after a server reboot
(`sonicpi reboot`).

## Everything else

| Command | What it does |
| --- | --- |
| `sonicpi volume [0-100]` | show or set the volume (shared with the desktop) |
| `sonicpi mixer --mono / --stereo / --invert / --drive PCT` | saved mixer settings |
| `sonicpi mixer --lpf 80 --hpf 30 --slide 4 / --no-lpf / --reset / --limiter` | the main mixer's filters |
| `sonicpi record out.wav [--duration 30s]` | record the output (or `record start`, `stop`, `save FILE`, `discard`) |
| `sonicpi midi [ports\|enable\|disable\|reset\|channel N]` | MIDI |
| `sonicpi osc [status\|enable\|disable\|public\|private]` | the OSC cue port |
| `sonicpi osc send /trigger/drums 1 2.5 hello` | send an OSC message to the cue port |
| `sonicpi link [status\|enable\|disable\|bpm 120]` | Ableton Link |
| `sonicpi timewarp 0.1` | shift all scheduling (the desktop's timewarp) |
| `sonicpi buffer [list\|show N\|run N\|edit N\|load N FILE\|save N FILE]` | the desktop's ten buffers |
| `sonicpi status` | server state, version, ports, audio and MIDI devices |
| `sonicpi reboot` | reboot the server (to pick up audio settings and `init.rb`) |
| `sonicpi version [--check]` | versions, and whether a newer Sonic Pi exists |
| `sonicpi doctor` | check the installation |
| `sonicpi completion bash\|zsh\|fish\|powershell` | shell completion |

Shared flags go before or after the command, on the commands that use
them (each command's `--help` lists its own): `--watch`, `--json`,
`--no-color` (or `NO_COLOR=1`), `--quiet`, and `--no-autostart` (fail
instead of starting the service).

Not included, on purpose: the built-in docs, tutorial and examples, and
GUI features such as the scopes. Those are in the desktop app and at
[sonic-pi.net](https://sonic-pi.net). The linter flags the two language
functions that need the desktop editor (`load_buffer` and `load_example`).

## The background service

Sonic Pi has to keep running to play code, so `sonicpid` runs it in the
background:

- **Linux**: a systemd user unit, `sonicpid.service`, in your login
  session, where your audio is. The .deb enables it for every user.
  `sonicpi service install` sets it up for tarball installs.
- **Windows**: the `sonicpid` Windows service, if you chose it in the
  installer (or later ran `sonicpi service install` as administrator). It
  runs as LocalSystem, with the installing user's `~/.sonic-pi` and
  `%TEMP%\sonic-pi`.
- **Neither**: the first `sonicpi` command that needs the server starts
  `sonicpid` as an ordinary background process.

```sh
sonicpi service status
sonicpi service start | stop | restart
sonicpi service install | uninstall
sonicpi service run           # in the foreground, printing everything
```

## How it works

```
 sonicpi (CLI) ──JSON over 127.0.0.1──► sonicpid (service)
                                           │  boots, keeps alive, listens on the GUI port
                                           ▼
                             Sonic Pi's boot daemon (daemon.rb)
                              ├── Spider: the Ruby language server
                              └── SuperSonic: the audio engine
```

- `sonicpid` starts Sonic Pi's own boot daemon from the bundled tree and
  then does what the desktop GUI does: it sends keep-alives and
  heartbeats, waits for the server to answer its pings, applies your
  preferences, and listens on the GUI port for everything the server
  reports.
- Those reports (logs, cues, errors, MIDI ports, Link) go to the session
  log and to any connected `sonicpi`. The CLI sends commands back over a
  local socket with a per-session token. The service publishes the socket
  in `$XDG_RUNTIME_DIR/sonic-pi-cli/` (Linux) or `%LOCALAPPDATA%` /
  `%ProgramData%\sonic-pi-cli\` (Windows).
- If the server dies, the service boots a new one.
- The linter and formatter are Ruby (`lint/`). They run on the same Ruby
  as the server and load Sonic Pi's own language modules for their
  knowledge.
- Sonic Pi 4.x trees work too: the service, settings and linter detect the
  version.

| Path | What it is |
| --- | --- |
| `cmd/sonicpi`, `internal/cli` | the CLI |
| `cmd/sonicpid`, `internal/service` | the service |
| `internal/osc`, `internal/events`, `internal/logfile`, `internal/prefs`, `internal/control`, `internal/svcmgr` | OSC; log events; the session log; the desktop's settings files; the CLI-service protocol; systemd and Windows service registration |
| `lint/` | the linter and formatter (`splint.rb`) and their tests |
| `packaging/` | the systemd unit, Debian scripts, tarball installer and NSIS installer |
| `scripts/` | building the server, staging, packaging, updating Sonic Pi |
| `external/sonic-pi` | Sonic Pi itself (submodule) |

## Building

You need Go 1.23+, Ruby 3.1+ (with headers), CMake, Ninja, a C/C++
compiler, Rust, and on Linux `libaubio-dev libasound2-dev
libjack-jackd2-dev libpipewire-0.3-dev libudev-dev libssl-dev`.

```sh
git clone --recurse-submodules -b claude/sonic-pi-cli https://github.com/AlexDev404/sonic-pi-lsp sonicpi-cli
cd sonicpi-cli
scripts/build-server-linux.sh   # Sonic Pi's Ruby extensions and audio engine
make build                      # build/bin/sonicpi and build/bin/sonicpid
make test
build/bin/sonicpi doctor
build/bin/sonicpi song.rb
```

Run from a checkout, `sonicpi` finds the server in `external/sonic-pi`.
`SONICPI_ROOT` points it at another tree. `SONIC_PI_HOME`,
`SONICPI_LOG_DIR` and `SONICPI_STATE_DIR` move the config, log and state
directories.

The Windows build is in [`.github/workflows/release.yml`](.github/workflows/release.yml).
It follows Sonic Pi's own CI: RubyInstaller with `rugged`, vcpkg for
libsndfile, Visual Studio for SuperSonic, then NSIS.

## Releases

[`.github/workflows/release.yml`](.github/workflows/release.yml) runs for
every pushed `v*` tag, and publishing a GitHub release creates its tag.
It builds:

- `sonicpi-cli_<version>_ubuntu-24.04_amd64.deb` and `…_ubuntu-22.04_amd64.deb`
- matching `.tar.gz` files for other distributions
- `sonicpi-cli_<version>_windows_x64_installer.exe` and a portable `.zip`
- `SHA256SUMS.txt`

and attaches them to the release for that tag. If the tag has no release
yet, the workflow creates one. To rebuild a tag's packages, run the
workflow by hand (Actions → Release → Run workflow).

```sh
git tag v1.0.0 && git push origin v1.0.0
```

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs the Go and
linter tests on Linux and Windows on every push.

## Updating Sonic Pi

```sh
scripts/update-sonic-pi.sh            # the newest Sonic Pi release
scripts/update-sonic-pi.sh v5.1.0     # or a given one
scripts/build-server-linux.sh && make test
git commit -am "Sonic Pi v5.1.0"
```

The linter's knowledge, the settings files and the server protocol all
come from the submodule, so a new release usually needs nothing else.

## Licence

This repository's code is under the AGPL-3.0-or-later, like the other
branches of this repository. Sonic Pi's licences are in its
[LICENSE.md](https://github.com/sonic-pi-net/sonic-pi/blob/dev/LICENSE.md).
