// Document symbols and go-to-definition for what the buffer defines:
// `define`d functions, live loops and named threads.
import { SymbolKind, type DocumentSymbol, type Range } from "vscode-languageserver";
import { wordAt } from "./context.js";
import type { Definition, DocumentInfo } from "./analysis.js";

const KIND: Record<Definition["kind"], SymbolKind> = {
  define: SymbolKind.Function,
  defonce: SymbolKind.Constant,
  live_loop: SymbolKind.Event,
  thread: SymbolKind.Event,
  cue: SymbolKind.Event,
};

export function nameRange(d: Definition): Range {
  return { start: { line: d.line, character: d.start }, end: { line: d.line, character: d.end } };
}

/** The `end` that closes the block a definition opens, found by indentation. */
function blockEnd(doc: DocumentInfo, d: Definition): number {
  const indent = /^\s*/.exec(doc.lines[d.line])![0].length;
  for (let i = d.line + 1; i < doc.lines.length; i++) {
    const m = /^(\s*)end\b/.exec(doc.lines[i]);
    if (m && m[1].length <= indent) return i;
  }
  return d.line;
}

export function documentSymbols(doc: DocumentInfo): DocumentSymbol[] {
  return doc.definitions.map((d) => {
    const last = blockEnd(doc, d);
    return {
      name: d.kind === "define" || d.kind === "defonce" ? d.name : `${d.kind} :${d.name}`,
      detail: d.kind,
      kind: KIND[d.kind],
      range: { start: { line: d.line, character: 0 }, end: { line: last, character: doc.lines[last].length } },
      selectionRange: nameRange(d),
    };
  });
}

/** Where the name under the cursor is defined in this buffer. */
export function definitionAt(doc: DocumentInfo, line: number, col: number): Definition | undefined {
  const w = wordAt(doc.lines[line] ?? "", col);
  if (!w) return undefined;
  if (w.kind === "word") return doc.functions.get(w.text);
  if (w.kind === "symbol") {
    const name = w.text.slice(1);
    return doc.definitions.find((d) => d.name === name && !(d.line === line && d.start === w.start + 1));
  }
  return undefined;
}
