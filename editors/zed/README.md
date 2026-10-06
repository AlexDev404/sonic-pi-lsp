# Sonic Pi for Zed

Runs [sonic-pi-lsp](https://github.com/AlexDev404/sonic-pi-lsp) for Ruby buffers in Zed:
docs on hover, completion, signature help and checks for Sonic Pi code.

## Install

1. Put `sonic-pi-lsp` on your PATH (from the repository root: `npm ci && npm run build && npm install -g .`),
   or point Zed at it in `settings.json`:

   ```json
   { "lsp": { "sonic-pi-lsp": { "binary": { "path": "/path/to/sonic-pi-lsp" } } } }
   ```

2. Install Zed's **Ruby** extension (the language this server attaches to), if you don't have it.

3. Install this folder as a dev extension: command palette → `zed: install dev extension` → choose
   `editors/zed`. Zed builds it (Rust, `wasm32-wasip1`; needs `rustup`).

Zed runs the server alongside its other Ruby servers. To use it on its own for Ruby:

```json
{ "languages": { "Ruby": { "language_servers": ["sonic-pi-lsp", "!solargraph", "!ruby-lsp", "..."] } } }
```
