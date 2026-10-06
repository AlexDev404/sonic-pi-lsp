# sonic-pi-lsp

A [Language Server Protocol](https://microsoft.github.io/language-server-protocol/) server for
[Sonic Pi](https://sonic-pi.net). It brings Sonic Pi's own documentation into any editor with an LSP
client (VS Code, Zed, JetBrains IDEs, Neovim, Helix, Emacs, Sublime Text, Kate, Vim): hover over
`play`, `:prophet`, `cutoff:` or `:bd_haus` to see what it is.

It speaks standard LSP (JSON-RPC over stdin/stdout), so there is one server for every editor. The
editor folders here are only thin client glue.

## What it does

**Hover**: what the thing under the cursor is.

| Hover over | Shows |
|---|---|
| `play`, `live_loop`, `with_fx` ... | usage, summary, full doc, opts, an example |
| `:prophet` after `use_synth` / `synth` | the synth's doc and its opts with defaults |
| `:reverb` after `with_fx` | the FX's doc and opts |
| `:bd_haus` after `sample` | the sample and its group |
| `cutoff:` | the opt **as its owner defines it**: the FX of the enclosing `with_fx`, the synth of a `synth` call, the current `use_synth` synth for `play`, the sampler for `sample`. Default, valid range, whether it slides |
| `:minor` in `chord :e3, :minor` | the intervals, and the notes from the given root (`:e3, :g3, :b3`) |
| `:e3`, or `60` in `play 60` | the note's name, MIDI number and frequency |
| your own `define :kick` | its parameters and the comment above it |
| `:beat` in `sync :beat` | the `live_loop :beat` it waits on |

For example, hovering `room:` in `with_fx :reverb, room: 0.8 do` shows:

> **`room:`** — opt of the :reverb FX
>
> The room size - a value between 0 (no reverb) and 1 (maximum reverb).
>
> - Default: `0.6`
> - Must be a value between 0 and 1 inclusively
> - Slidable: set `room_slide:` to glide to new values with `control`

**Completion** that knows the slot: synth names after `use_synth`, FX after `with_fx`, samples
after `sample`, chord and scale names, notes, tunings, random sources, and your own live loop and
cue names after `sync`. Past a call's first argument it offers that call's opts (the current synth's
for `play`), skipping ones already given. In an opt's value slot it offers the allowed values
(`wave: 0`/`1`/`2`) or the default.

**Signature help** for every function and your own `define`s, tracking the active argument into the
opts.

**Diagnostics** for what is certain from the source:
- unknown synth, FX, sample, chord, scale, tuning or random-source names, with a *did you mean*
  (synth names are left alone in buffers that `load_synthdefs`);
- opt values Sonic Pi would reject at run time (`room: 2`, `cutoff: 131`, `wave: 3`), checked against
  the same bounds Sonic Pi validates;
- opts a synth or FX doesn't take, which Sonic Pi silently ignores (shown as information).

**Document symbols** (outline) and **go to definition** for `define`s, live loops and named threads.

## Install

Requires Node.js 18 or newer.

```sh
git clone -b claude/sonic-pi-lsp-intellisense-nb3z4z https://github.com/AlexDev404/sonic-pi-lsp
cd sonic-pi-lsp
npm ci            # also builds dist/sonic-pi-lsp.cjs
npm install -g .  # puts `sonic-pi-lsp` on your PATH
sonic-pi-lsp --version
```

`dist/sonic-pi-lsp.cjs` is a single self-contained file (server, LSP library and all of Sonic Pi's
docs), so you can also copy it anywhere and run it with `node sonic-pi-lsp.cjs`.

```
sonic-pi-lsp            speak LSP over stdin/stdout (the default)
sonic-pi-lsp --stdio    the same, for clients that pass it
sonic-pi-lsp --socket=PORT | --pipe=NAME | --node-ipc
```

## Editor setup

Sonic Pi code is Ruby, so the server attaches to Ruby buffers. Save your Sonic Pi code as `.rb` (or
`.spi` mapped to Ruby).

- **VS Code**: the extension in [`editors/vscode`](editors/vscode) bundles the server.
- **Zed**: the extension in [`editors/zed`](editors/zed).
- **JetBrains IDEs, Neovim, Helix, Emacs, Sublime Text, Kate, Vim**: configuration only, see
  [`editors/README.md`](editors/README.md).

### Settings

- Turn diagnostics off with the initialization option `{ "diagnostics": false }`, or the workspace
  setting `sonicPi.diagnostics: false` (sent with `workspace/didChangeConfiguration`).

## How it works

`data/sonic-pi.json` holds everything the server knows: every function, synth, FX, sample, chord and
scale, with Markdown docs and the opts' bounds. [`tools/extract-docs.rb`](tools/extract-docs.rb)
extracts it from a Sonic Pi checkout by loading Sonic Pi's own Ruby doc system (the `doc name: ...`
entries in `app/server/ruby/lib/sonicpi/lang/` and `synthinfo.rb`), so it is the same text Sonic Pi's
help panel shows. To refresh it for a new Sonic Pi version:

```sh
ruby tools/extract-docs.rb /path/to/sonic-pi   # rewrites data/sonic-pi.json
npm test
```

The server reads code the way Sonic Pi's native editor does: [`src/context.ts`](src/context.ts) is a
port of the GUI's completion engine (`app/gui/utils/completion_context.cpp`), which finds the innermost
call around the cursor, skips strings and comments, and tells positional arguments from opts. It is
extended to follow a call whose opts continue onto the next lines.

| File | Role |
|---|---|
| `src/server.ts` | LSP wiring and the command line |
| `src/context.ts` | where the cursor is: string/comment, call, argument, token |
| `src/analysis.ts` | what a buffer defines; whose opts a call takes; what kind of name a slot takes |
| `src/hover.ts`, `completion.ts`, `signature.ts`, `diagnostics.ts`, `navigation.ts` | the features |
| `src/markdown.ts` | how docs are rendered |
| `src/data.ts`, `data/sonic-pi.json` | Sonic Pi's facts |

## Limitations

- Each buffer is read on its own; `run_file` and `load` are not followed.
- The synth `play` uses is the last `use_synth` / `with_synth` above it in the file, which is what
  you mean in almost every buffer but not a guarantee across threads.
- Checks only cover literal values. Computed values (`cutoff: rrand(60, 140)`) are not evaluated.

## Development

```sh
npm ci
npm test         # typecheck, bundle, and run the tests (unit tests and the bundled server over stdio)
npm run build    # build/ (compiled) and dist/sonic-pi-lsp.cjs (bundle)
```

## License

MIT. `data/sonic-pi.json` is extracted from Sonic Pi's documentation, which is MIT licensed,
© Samuel Aaron and contributors. See [LICENSE](LICENSE).
