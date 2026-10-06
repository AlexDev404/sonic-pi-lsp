// The language server: Sonic Pi's docs, completion and checks over the
// Language Server Protocol, for any editor with an LSP client.
//
//   sonic-pi-lsp            speak LSP over stdin/stdout (the default)
//   sonic-pi-lsp --stdio    the same, for clients that pass it
//   sonic-pi-lsp --socket=PORT | --node-ipc | --pipe=NAME
import {
  createConnection, ProposedFeatures, TextDocuments, TextDocumentSyncKind, MarkupKind,
  type InitializeParams, type InitializeResult,
} from "vscode-languageserver/node";
import { TextDocument } from "vscode-languageserver-textdocument";
import { analyse, type DocumentInfo } from "./analysis.js";
import { hover } from "./hover.js";
import { completions, resolveCompletion } from "./completion.js";
import { signatureHelp } from "./signature.js";
import { diagnostics } from "./diagnostics.js";
import { definitionAt, documentSymbols, nameRange } from "./navigation.js";
import { data } from "./data.js";

declare const SONIC_PI_LSP_VERSION: string | undefined;
const VERSION = typeof SONIC_PI_LSP_VERSION === "string" ? SONIC_PI_LSP_VERSION : "dev";

const args = process.argv.slice(2);
if (args.includes("--version") || args.includes("-v")) {
  process.stdout.write(`sonic-pi-lsp ${VERSION} (Sonic Pi ${data.sonicPiVersion} docs)\n`);
  process.exit(0);
}
if (args.includes("--help") || args.includes("-h")) {
  process.stdout.write(
    "Usage: sonic-pi-lsp [--stdio | --socket=PORT | --node-ipc | --pipe=NAME]\n\n" +
    "A Language Server Protocol server for Sonic Pi. With no transport flag it\n" +
    "speaks LSP over stdin/stdout. Point your editor's LSP client at this command.\n",
  );
  process.exit(0);
}
if (!args.some((a) => /^--(stdio|node-ipc|socket|pipe)/.test(a))) process.argv.push("--stdio");

const connection = createConnection(ProposedFeatures.all);
const documents = new TextDocuments(TextDocument);
const analysed = new Map<string, { version: number; info: DocumentInfo }>();
let diagnosticsEnabled = true;

function info(uri: string): DocumentInfo | undefined {
  const doc = documents.get(uri);
  if (!doc) return undefined;
  const cached = analysed.get(uri);
  if (cached && cached.version === doc.version) return cached.info;
  const fresh = analyse(doc.getText());
  analysed.set(uri, { version: doc.version, info: fresh });
  return fresh;
}

connection.onInitialize((params: InitializeParams): InitializeResult => {
  const opts = params.initializationOptions as { diagnostics?: boolean } | undefined;
  if (opts?.diagnostics === false) diagnosticsEnabled = false;
  return {
    capabilities: {
      textDocumentSync: TextDocumentSyncKind.Incremental,
      hoverProvider: true,
      completionProvider: { triggerCharacters: [":"], resolveProvider: true },
      signatureHelpProvider: { triggerCharacters: [" ", ","], retriggerCharacters: [","] },
      definitionProvider: true,
      documentSymbolProvider: true,
    },
    serverInfo: { name: "sonic-pi-lsp", version: VERSION },
  };
});

connection.onDidChangeConfiguration((change) => {
  const enabled = (change.settings as { sonicPi?: { diagnostics?: boolean } } | undefined)?.sonicPi?.diagnostics;
  if (typeof enabled === "boolean" && enabled !== diagnosticsEnabled) {
    diagnosticsEnabled = enabled;
    documents.all().forEach(validate);
  }
});

function validate(doc: TextDocument): void {
  const i = info(doc.uri);
  if (!i) return;
  void connection.sendDiagnostics({ uri: doc.uri, version: doc.version, diagnostics: diagnosticsEnabled ? diagnostics(i) : [] });
}

documents.onDidChangeContent((e) => validate(e.document));
documents.onDidClose((e) => {
  analysed.delete(e.document.uri);
  void connection.sendDiagnostics({ uri: e.document.uri, diagnostics: [] });
});

connection.onHover(({ textDocument, position }) => {
  const i = info(textDocument.uri);
  const h = i && hover(i, position.line, position.character);
  if (!h) return null;
  return {
    contents: { kind: MarkupKind.Markdown, value: h.markdown },
    range: { start: { line: h.line, character: h.start }, end: { line: h.line, character: h.end } },
  };
});

connection.onCompletion(({ textDocument, position }) => {
  const i = info(textDocument.uri);
  return i ? completions(i, position.line, position.character) : [];
});

connection.onCompletionResolve(resolveCompletion);

connection.onSignatureHelp(({ textDocument, position }) => {
  const i = info(textDocument.uri);
  return i ? signatureHelp(i, position.line, position.character) : null;
});

connection.onDefinition(({ textDocument, position }) => {
  const i = info(textDocument.uri);
  const d = i && definitionAt(i, position.line, position.character);
  return d ? { uri: textDocument.uri, range: nameRange(d) } : null;
});

connection.onDocumentSymbol(({ textDocument }) => {
  const i = info(textDocument.uri);
  return i ? documentSymbols(i) : [];
});

documents.listen(connection);
connection.listen();
