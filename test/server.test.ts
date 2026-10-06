// The bundled server, driven over stdio with plain JSON-RPC the way any
// editor's LSP client drives it.
import { test, after } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";

const bin = fileURLToPath(new URL("../../dist/sonic-pi-lsp.cjs", import.meta.url));

class Client {
  private proc = spawn(process.execPath, [bin], { stdio: ["pipe", "pipe", "inherit"] });
  private buf = Buffer.alloc(0);
  private nextId = 0;
  private pending = new Map<number, (result: unknown) => void>();
  private listeners: ((msg: { method: string; params: unknown }) => void)[] = [];

  constructor() {
    this.proc.stdout.on("data", (chunk: Buffer) => {
      this.buf = Buffer.concat([this.buf, chunk]);
      for (;;) {
        const header = this.buf.indexOf("\r\n\r\n");
        if (header < 0) return;
        const length = Number(/Content-Length: (\d+)/i.exec(this.buf.subarray(0, header).toString())![1]);
        if (this.buf.length < header + 4 + length) return;
        const msg = JSON.parse(this.buf.subarray(header + 4, header + 4 + length).toString());
        this.buf = this.buf.subarray(header + 4 + length);
        if (msg.id !== undefined && this.pending.has(msg.id)) {
          this.pending.get(msg.id)!(msg.result);
          this.pending.delete(msg.id);
        } else if (msg.method) {
          this.listeners.forEach((l) => l(msg));
        }
      }
    });
  }

  private send(msg: object): void {
    const body = JSON.stringify({ jsonrpc: "2.0", ...msg });
    this.proc.stdin.write(`Content-Length: ${Buffer.byteLength(body)}\r\n\r\n${body}`);
  }

  request<T>(method: string, params: unknown): Promise<T> {
    const id = ++this.nextId;
    return new Promise((resolve) => {
      this.pending.set(id, resolve as (r: unknown) => void);
      this.send({ id, method, params });
    });
  }

  notify(method: string, params: unknown): void {
    this.send({ method, params });
  }

  next<T>(method: string): Promise<T> {
    return new Promise((resolve) => {
      const l = (msg: { method: string; params: unknown }) => {
        if (msg.method !== method) return;
        this.listeners = this.listeners.filter((x) => x !== l);
        resolve(msg.params as T);
      };
      this.listeners.push(l);
    });
  }

  async close(): Promise<void> {
    await this.request("shutdown", null);
    this.notify("exit", null);
    await new Promise((r) => this.proc.on("exit", r));
  }
}

const client = new Client();
after(() => client.close());
const uri = "file:///tmp/song.rb";

test("initialize advertises the features", async () => {
  const init = await client.request<{ capabilities: Record<string, unknown>; serverInfo: { name: string } }>(
    "initialize", { processId: null, rootUri: null, capabilities: {} },
  );
  client.notify("initialized", {});
  assert.equal(init.serverInfo.name, "sonic-pi-lsp");
  for (const cap of ["hoverProvider", "completionProvider", "signatureHelpProvider", "definitionProvider", "documentSymbolProvider"]) {
    assert.ok(init.capabilities[cap], cap);
  }
});

test("opening a document publishes its diagnostics", async () => {
  const published = client.next<{ uri: string; diagnostics: { message: string }[] }>("textDocument/publishDiagnostics");
  client.notify("textDocument/didOpen", {
    textDocument: { uri, languageId: "ruby", version: 1, text: "use_synth :prohpet\nlive_loop :beat do\n  play 60\n  sleep 1\nend\n" },
  });
  const p = await published;
  assert.equal(p.uri, uri);
  assert.deepEqual(p.diagnostics.map((d) => d.message), ["Unknown synth :prohpet (did you mean :prophet?)"]);
});

test("edits re-check the document", async () => {
  const published = client.next<{ diagnostics: unknown[] }>("textDocument/publishDiagnostics");
  client.notify("textDocument/didChange", {
    textDocument: { uri, version: 2 },
    contentChanges: [{ range: { start: { line: 0, character: 11 }, end: { line: 0, character: 18 } }, text: "prophet" }],
  });
  assert.deepEqual((await published).diagnostics, []);
});

test("hover", async () => {
  const h = await client.request<{ contents: { kind: string; value: string } }>("textDocument/hover", { textDocument: { uri }, position: { line: 0, character: 13 } });
  assert.equal(h.contents.kind, "markdown");
  assert.match(h.contents.value, /The Prophet/);
});

test("completion and resolve", async () => {
  const items = await client.request<{ label: string; data?: unknown }[]>("textDocument/completion", { textDocument: { uri }, position: { line: 0, character: 11 } });
  const prophet = items.find((i) => i.label === ":prophet")!;
  assert.ok(prophet);
  const resolved = await client.request<{ documentation: { value: string } }>("completionItem/resolve", prophet);
  assert.match(resolved.documentation.value, /Pulse Width Modulation/);
});

test("definition and document symbols", async () => {
  const symbols = await client.request<{ name: string }[]>("textDocument/documentSymbol", { textDocument: { uri } });
  assert.deepEqual(symbols.map((s) => s.name), ["live_loop :beat"]);
});
