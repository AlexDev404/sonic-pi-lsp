# Editor setup

Every editor runs the same server: `sonic-pi-lsp --stdio` (see the [main README](../README.md) for
installing it). If it isn't on your PATH, use `node /path/to/dist/sonic-pi-lsp.cjs --stdio` as the
command instead.

The server attaches to Ruby buffers, since Sonic Pi code is Ruby. Most setups below enable it
alongside your other Ruby servers. If you only write Sonic Pi code in a project, you can make it the
only one.

## VS Code

Install the extension in [`vscode/`](vscode). It bundles the server. See its README for building
the `.vsix`.

## Zed

Install [`zed/`](zed) as a dev extension (command palette → `zed: install dev extension`). It runs
`sonic-pi-lsp` from your PATH, or from `lsp.sonic-pi-lsp.binary.path` in your settings.

## JetBrains IDEs (IntelliJ IDEA, RubyMine, WebStorm, PyCharm, ...)

Use [LSP4IJ](https://plugins.jetbrains.com/plugin/23257-lsp4ij), Red Hat's free LSP client plugin. It
works in the Community editions too.

1. **Settings → Plugins → Marketplace**, install **LSP4IJ**, restart.
2. **Settings → Languages & Frameworks → Language Servers**, click **+**.
3. **Server** tab: name `Sonic Pi`, command `sonic-pi-lsp --stdio`.
4. **Mappings** tab → **File name patterns**: add `*.rb` and `*.spi`. In RubyMine you can map the
   **Ruby** language instead.
5. **OK**, then open a Sonic Pi file. The LSP4IJ tool window shows the server's status and traces.

## Neovim (0.11+)

```lua
vim.lsp.config("sonic_pi", {
  cmd = { "sonic-pi-lsp", "--stdio" },
  filetypes = { "ruby" },
  root_markers = { ".git" },
})
vim.lsp.enable("sonic_pi")
```

Hover is `K`, completion is `<C-x><C-o>` (or your completion plugin), and diagnostics show inline.

For `.spi` files: `vim.filetype.add({ extension = { spi = "ruby" } })`.

## Helix

`~/.config/helix/languages.toml`:

```toml
[language-server.sonic-pi-lsp]
command = "sonic-pi-lsp"
args = ["--stdio"]

[[language]]
name = "ruby"
language-servers = ["sonic-pi-lsp", "ruby-lsp", "solargraph"]
file-types = ["rb", "spi"]
```

## Emacs

With Eglot (built in since Emacs 29):

```elisp
(with-eval-after-load 'eglot
  (add-to-list 'eglot-server-programs
               '((ruby-mode ruby-ts-mode) . ("sonic-pi-lsp" "--stdio"))))
(add-to-list 'auto-mode-alist '("\\.spi\\'" . ruby-mode))
```

Then `M-x eglot` in a Sonic Pi buffer.

## Sublime Text

Install the [LSP](https://packagecontrol.io/packages/LSP) package, then in
**Preferences → Package Settings → LSP → Settings**:

```json
{
  "clients": {
    "sonic-pi-lsp": {
      "enabled": true,
      "command": ["sonic-pi-lsp", "--stdio"],
      "selector": "source.ruby"
    }
  }
}
```

## Kate

**Settings → Configure Kate → LSP Client → User Server Settings**:

```json
{
  "servers": {
    "ruby": {
      "command": ["sonic-pi-lsp", "--stdio"],
      "highlightingModeRegex": "^Ruby$"
    }
  }
}
```

## Vim

With [vim-lsp](https://github.com/prabirshrestha/vim-lsp):

```vim
au User lsp_setup call lsp#register_server({
  \ 'name': 'sonic-pi-lsp',
  \ 'cmd': {server_info -> ['sonic-pi-lsp', '--stdio']},
  \ 'allowlist': ['ruby'],
  \ })
```

## Any other editor

Point its LSP client at `sonic-pi-lsp --stdio` for Ruby files. The server advertises hover,
completion (with `completionItem/resolve`), signature help, definition, document symbols and
diagnostics. It takes one initialization option, `{ "diagnostics": false }`, to turn checks off.
