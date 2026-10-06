// Completion: what can be typed at the cursor. Follows the native GUI's
// rules (app/gui/utils/scintilla_api.cpp): a slot's kind (synth, sample,
// chord ...) decides the names offered, past a call's first argument come its
// opts, and an opt's value slot offers that opt's values.
import {
  CompletionItemKind, MarkupKind, type CompletionItem, type Range,
} from "vscode-languageserver";
import { data, own, type Opt } from "./data.js";
import {
  caretAfterClosedValue, isOptKey, lineToContext, logicalLine, scanLineToCaret, tokenEndAtCaret, wordsBefore,
} from "./context.js";
import { optOwner, slotKind, type DocumentInfo, type OptOwner } from "./analysis.js";
import {
  chordMarkdown, constraintText, definitionMarkdown, functionMarkdown, instrumentMarkdown, optMarkdown,
  sampleMarkdown, scaleMarkdown,
} from "./markdown.js";
import { describeMidi, noteSpellings } from "./notes.js";

/** What `completionItem/resolve` needs to fill in an item's documentation. */
export interface ResolveData {
  t: "fn" | "synth" | "fx" | "sample" | "chord" | "scale";
  n: string;
}

const md = (value: string) => ({ kind: MarkupKind.Markdown, value });

function firstLine(s: string): string {
  const t = s.replace(/\s+/g, " ").trim();
  return (/^.+?[.!?](?=\s|$)/.exec(t)?.[0] ?? t).slice(0, 100);
}

export function completions(doc: DocumentInfo, line: number, col: number): CompletionItem[] {
  const text = doc.lines[line];
  if (text === undefined) return [];
  const scan = scanLineToCaret(text, col);
  if (scan.inComment || scan.inString || caretAfterClosedValue(text, col)) return [];

  const logical = logicalLine(doc.lines, line, col);
  const context = lineToContext(logical.text, logical.col);
  const partial = context[context.length - 1] ?? "";
  const words = wordsBefore(context);
  const last = words[words.length - 1];
  const end = tokenEndAtCaret(text, col);
  const range: Range = { start: { line, character: Math.max(0, end - partial.length) }, end: { line, character: end } };

  // Just typed an opt key's colon: the value comes after a space.
  if (isOptKey(partial)) return [];

  const symbols = (names: string[], kind: CompletionItemKind, detail: (n: string) => string | undefined, t?: ResolveData["t"]): CompletionItem[] =>
    names.map((n) => ({
      label: `:${n}`,
      kind,
      detail: detail(n),
      filterText: partial.startsWith(":") || partial === "" ? `:${n}` : n,
      textEdit: { range, newText: `:${n}` },
      data: t ? ({ t, n } satisfies ResolveData) : undefined,
    }));

  const notes = (): CompletionItem[] =>
    noteSpellings().map(({ symbol, midi }) => ({
      label: symbol,
      kind: CompletionItemKind.Value,
      detail: describeMidi(midi),
      sortText: String(midi).padStart(3, "0") + symbol,
      filterText: partial.startsWith(":") || partial === "" ? symbol : symbol.slice(1),
      textEdit: { range, newText: symbol },
    }));

  const cues = (): CompletionItem[] => symbols([...doc.cueNames].sort(), CompletionItemKind.Event, () => "cue");

  const byKind = (kind: string | undefined): CompletionItem[] | undefined => {
    switch (kind) {
      case "synth": return symbols(Object.keys(data.synths), CompletionItemKind.Class, (n) => data.synths[n].title, "synth");
      case "fx": return symbols(Object.keys(data.fx), CompletionItemKind.Module, (n) => data.fx[n].title, "fx");
      case "sample": return symbols(Object.keys(data.samples), CompletionItemKind.File, (n) => data.samples[n], "sample");
      case "chord": return symbols(Object.keys(data.chords).filter((n) => /^[\p{L}_][\p{L}\p{N}_]*$/u.test(n)), CompletionItemKind.Constant, (n) => `chord: ${data.chords[n].join(" ")}`, "chord");
      case "scale": return symbols(Object.keys(data.scales), CompletionItemKind.Constant, (n) => `scale: ${data.scales[n].join(" ")}`, "scale");
      case "note": return notes();
      case "cue": return cues();
      case "tuning": return symbols(data.tunings, CompletionItemKind.Constant, () => "tuning");
      case "random_source": return symbols(data.randomSources, CompletionItemKind.Constant, () => "random source");
      case "track": case "link_peer": case "link_channel": return [];
    }
    return undefined;
  };

  // An opt's value.
  if (last && isOptKey(last)) {
    const fromKind = byKind(slotKind(context));
    if (fromKind) return fromKind;
    const owner = optOwner(context, doc.lines, line, col);
    const opt = owner && own(owner.opts, last.slice(0, -1));
    if (!opt) return [];
    const values = opt.options ?? (opt.default !== undefined ? [opt.default] : []);
    return values.map((v, i) => ({
      label: String(v),
      kind: opt.options ? CompletionItemKind.EnumMember : CompletionItemKind.Value,
      detail: opt.options ? undefined : ["default", constraintText(opt)].filter(Boolean).join(" · "),
      documentation: md(optMarkdown(last, opt, owner?.label)),
      sortText: String(i).padStart(3, "0"),
      textEdit: { range, newText: String(v) },
    }));
  }

  // A positional argument that names something.
  const fromKind = byKind(slotKind(context));
  if (fromKind) return fromKind;

  // Past a call's first argument: its opts.
  const owner = optOwner(context, doc.lines, line, col);
  if (owner && pastFirstArgument(words, owner)) return optKeys(owner, words, range);

  if (partial.startsWith(":")) return cues();

  // Anything else: a function.
  const items: CompletionItem[] = Object.entries(data.functions).map(([name, fn]) => ({
    label: name,
    kind: CompletionItemKind.Function,
    detail: fn.summary,
    textEdit: { range, newText: name },
    data: { t: "fn", n: name } satisfies ResolveData,
  }));
  for (const def of doc.functions.values()) {
    if (own(data.functions, def.name)) continue;
    items.push({
      label: def.name,
      kind: CompletionItemKind.Function,
      detail: "user-defined function",
      documentation: md(definitionMarkdown(def)),
      textEdit: { range, newText: def.name },
    });
  }
  return items;
}

/** True once the call's own first argument (a synth, FX, sample or loop name) has been given, or it takes none. */
function pastFirstArgument(words: string[], owner: OptOwner): boolean {
  const args = own(data.functions, owner.fn)?.args.length ?? 1;
  return words.length - 1 > owner.index || args === 0;
}

function optKeys(owner: OptOwner, words: string[], range: Range): CompletionItem[] {
  const used = new Set(words.filter(isOptKey));
  return Object.entries(owner.opts)
    .filter(([k]) => !used.has(`${k}:`))
    .map(([k, opt]: [string, Opt], i) => ({
      label: `${k}:`,
      kind: CompletionItemKind.Property,
      detail: opt.default !== undefined ? `default ${opt.default}` : firstLine(opt.doc),
      documentation: md(optMarkdown(k, opt, owner.label)),
      // slide variants after their opt
      sortText: (/_slide(_shape|_curve)?$/.test(k) ? "1" : "0") + String(i).padStart(3, "0"),
      textEdit: { range, newText: `${k}: ` },
    }));
}

/** Fill in the documentation the list leaves out to stay small. */
export function resolveCompletion(item: CompletionItem): CompletionItem {
  const d = item.data as ResolveData | undefined;
  if (!d) return item;
  let value: string | undefined;
  switch (d.t) {
    case "fn": { const fn = own(data.functions, d.n); if (fn) value = functionMarkdown(d.n, fn); break; }
    case "synth": { const s = own(data.synths, d.n); if (s) value = instrumentMarkdown(d.n, s, "synth"); break; }
    case "fx": { const f = own(data.fx, d.n); if (f) value = instrumentMarkdown(d.n, f, "fx"); break; }
    case "sample": value = sampleMarkdown(d.n); break;
    case "chord": { const c = own(data.chords, d.n); if (c) value = chordMarkdown(d.n, c); break; }
    case "scale": { const s = own(data.scales, d.n); if (s) value = scaleMarkdown(d.n, s); break; }
  }
  return value ? { ...item, documentation: md(value) } : item;
}
