# Sonic Pi for Zed

Runs [sonic-pi-lsp](https://github.com/AlexDev404/sonic-pi-lsp) for Ruby buffers in Zed:
docs on hover, completion, signature help and checks for Sonic Pi code.

## 1. Give Zed the server

The extension starts `sonic-pi-lsp` from your PATH. Either:

- **Install it on the PATH** (needs Node.js 18+ and npm): `npm install -g sonic-pi-lsp-0.1.0.tgz`
  (the tarball in the CI's `sonic-pi-lsp` artifact), or from a clone of the repository
  `npm ci && npm install -g .`. Check with `sonic-pi-lsp --version`.
- **Or point Zed at the single-file server** (`sonic-pi-lsp.cjs` from the same artifact) in Zed's
  `settings.json`, with full paths:

  ```json
  {
    "lsp": {
      "sonic-pi-lsp": {
        "binary": {
          "path": "/usr/local/bin/node",
          "arguments": ["/path/to/sonic-pi-lsp.cjs", "--stdio"]
        }
      }
    }
  }
  ```

  `path` is your Node executable (`which node`, or `where node` on Windows).

## 2. Install the extension

You also need Zed's **Ruby** extension, the language this server attaches to.

- **From source** (needs Rust via `rustup`): command palette → `zed: install dev extension` → choose
  this `editors/zed` folder. Zed compiles it.
- **Pre-built** (the CI's `sonic-pi-zed-extension` artifact): unzip it into a folder named `sonic-pi`
  inside Zed's installed extensions directory. Zed picks it up as it appears:
  - macOS: `~/Library/Application Support/Zed/extensions/installed/sonic-pi/`
  - Linux: `~/.local/share/zed/extensions/installed/sonic-pi/`
  - Windows: `%LOCALAPPDATA%\Zed\extensions\installed\sonic-pi\`

  The folder holds `extension.toml` and `extension.wasm`. Don't install it with
  `zed: install dev extension`: that command builds from source and needs the `Cargo.toml`.

## 3. Check it's running

Open a `.rb` file of Sonic Pi code and hover over `play` or `:prophet`. If nothing shows, open the
command palette → `dev: open language server logs` and pick **Sonic Pi**. The error there says
whether Zed couldn't find `sonic-pi-lsp`.

Zed runs the server alongside its other Ruby servers. To use it on its own for Ruby:

```json
{ "languages": { "Ruby": { "language_servers": ["sonic-pi-lsp", "!solargraph", "!ruby-lsp", "..."] } } }
```
