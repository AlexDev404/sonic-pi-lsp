// Starts sonic-pi-lsp for Ruby files (and .spi files, which this extension
// registers as Ruby). The server is the same one every other editor uses;
// this is only the client glue.
const path = require("node:path");
const vscode = require("vscode");
const { LanguageClient, TransportKind } = require("vscode-languageclient/node");

let client;

exports.activate = async function activate(context) {
  const config = vscode.workspace.getConfiguration("sonicPi");
  const custom = config.get("serverPath");
  const serverOptions = custom
    ? { command: custom, args: ["--stdio"] }
    : { module: context.asAbsolutePath(path.join("server", "sonic-pi-lsp.cjs")), transport: TransportKind.stdio };

  client = new LanguageClient("sonicPi", "Sonic Pi", serverOptions, {
    documentSelector: [{ language: "ruby" }],
    initializationOptions: { diagnostics: config.get("diagnostics") },
    synchronize: { configurationSection: "sonicPi" },
  });
  await client.start();
};

exports.deactivate = function deactivate() {
  return client ? client.stop() : undefined;
};
