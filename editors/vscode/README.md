# Sonic Pi Language Support

Hover docs, completion, signature help and checks for [Sonic Pi](https://sonic-pi.net)
code in VS Code, from [sonic-pi-lsp](https://github.com/AlexDev404/sonic-pi-lsp).

The extension starts the language server for Ruby files, and registers `.spi` as Ruby.

## Settings

- `sonicPi.serverPath`: use your own `sonic-pi-lsp` executable instead of the bundled one.
- `sonicPi.diagnostics`: turn the checks off (`false`) and keep hover and completion.

## Building

```sh
# in the repository root
npm ci && npm run build
# then here
cd editors/vscode
npm install
npm run package        # writes sonic-pi-lsp-<version>.vsix
code --install-extension sonic-pi-lsp-*.vsix
```
