// Hover: what the thing under the cursor is.
import { data, own } from "./data.js";
import { lineToContext, logicalLine, wordAt, wordsBefore } from "./context.js";
import { lookupOpt, optOwner, slotKind, type Definition, type DocumentInfo } from "./analysis.js";
import { describeMidi, noteToMidi } from "./notes.js";
import {
  chordMarkdown, definitionMarkdown, functionMarkdown, instrumentMarkdown, noteMarkdown, optMarkdown,
  sampleMarkdown, scaleMarkdown,
} from "./markdown.js";

export interface HoverResult {
  markdown: string;
  line: number;
  start: number;
  end: number;
}

/** The note a chord or scale is built on: the argument after `chord` / `scale`. */
function tonicOf(words: string[], fn: string): number | undefined {
  const i = words.lastIndexOf(fn);
  const w = i >= 0 ? words[i + 1] : undefined;
  if (w === undefined) return undefined;
  if (/^\d+(\.\d+)?$/.test(w)) return Number(w);
  return noteToMidi(w);
}

const userDefinition = (doc: DocumentInfo, name: string, kinds: Definition["kind"][]): Definition | undefined =>
  doc.definitions.find((d) => d.name === name && kinds.includes(d.kind));

export function symbolMarkdown(doc: DocumentInfo, name: string, kind: string | undefined, words: string[]): string | undefined {
  switch (kind) {
    case "synth": { const s = own(data.synths, name); if (s) return instrumentMarkdown(name, s, "synth"); break; }
    case "fx": { const f = own(data.fx, name); if (f) return instrumentMarkdown(name, f, "fx"); break; }
    case "sample": if (own(data.samples, name)) return sampleMarkdown(name); break;
    case "chord": { const c = own(data.chords, name); if (c) return chordMarkdown(name, c, tonicOf(words, "chord")); break; }
    case "scale": { const s = own(data.scales, name); if (s) return scaleMarkdown(name, s, tonicOf(words, "scale")); break; }
    case "note": { const m = noteToMidi(name); if (m !== undefined) return noteMarkdown(`:${name}`, m); break; }
    case "cue": {
      const def = userDefinition(doc, name, ["live_loop", "thread"]);
      if (def) return definitionMarkdown(def);
      if (doc.cueNames.has(name)) return `**:${name}** — cue, sent with \`cue :${name}\` or \`set :${name}\` in this buffer`;
      break;
    }
    case "tuning": if (data.tunings.includes(name)) return `**:${name}** — tuning system\n\nSee \`use_tuning\`.`; break;
    case "random_source": if (data.randomSources.includes(name)) return `**:${name}** — random number source\n\nSee \`use_random_source\`.`; break;
  }
  // Not in a slot that says what it is: whichever name it matches.
  const def = userDefinition(doc, name, ["define", "defonce", "live_loop", "thread"]);
  if (def) return definitionMarkdown(def);
  for (const k of ["synth", "fx", "sample", "note", "chord", "scale"]) {
    if (k === kind) continue;
    const md = symbolMarkdown(doc, name, k, words);
    if (md) return md;
  }
  return undefined;
}

export function hover(doc: DocumentInfo, line: number, col: number): HoverResult | undefined {
  const text = doc.lines[line];
  if (text === undefined) return undefined;
  const w = wordAt(text, col);
  if (!w) return undefined;
  const logical = logicalLine(doc.lines, line, w.end);
  const context = lineToContext(logical.text, logical.col);
  const words = wordsBefore(context);
  const result = (markdown: string | undefined): HoverResult | undefined =>
    markdown ? { markdown, line, start: w.start, end: w.end } : undefined;

  switch (w.kind) {
    case "key": {
      const owner = optOwner(context, doc.lines, line, w.start);
      const opt = lookupOpt(owner, w.text);
      return result(opt && optMarkdown(w.text, opt, owner?.label));
    }
    case "symbol":
      return result(symbolMarkdown(doc, w.text.slice(1), slotKind(context), words));
    case "number": {
      const n = Number(w.text);
      if (!Number.isFinite(n)) return undefined;
      if (slotKind(context) === "note") return result(noteMarkdown(w.text, n));
      const last = words[words.length - 1];
      if (last?.endsWith(":")) {
        const opt = lookupOpt(optOwner(context, doc.lines, line, w.start), last);
        if (opt?.midi) return result(`**${w.text}** — ${describeMidi(n)}`);
      }
      return undefined;
    }
    case "word": {
      const fn = own(data.functions, w.text);
      if (fn) return result(functionMarkdown(w.text, fn));
      const def = doc.functions.get(w.text);
      return result(def && definitionMarkdown(def));
    }
  }
}
