// What a Sonic Pi buffer defines, and what its calls mean at a position.
import { argKinds, data, own, PLAY_FNS, SAMPLE_FNS, type Opt } from "./data.js";
import { callSlot, codeEnd, isOptKey, wordsBefore } from "./context.js";

export type DefinitionKind = "define" | "defonce" | "live_loop" | "thread" | "cue";

export interface Definition {
  name: string;
  kind: DefinitionKind;
  line: number;
  start: number;
  end: number;
  /** Block parameters, for `define :foo do |a, b|`. */
  params: string[];
  /** The comment lines directly above it. */
  doc?: string;
}

export interface DocumentInfo {
  lines: string[];
  definitions: Definition[];
  /** User functions by name (`define` / `defonce`). */
  functions: Map<string, Definition>;
  /** Names a `sync` can wait on: live loops, threads, and `cue`/`set` names. */
  cueNames: Set<string>;
  /** True when the buffer loads its own synthdefs, so unknown synth names are expected. */
  loadsSynthdefs: boolean;
}

const NAME = String.raw`([\p{L}_][\p{L}\p{N}_]*[?!]?)`;
const DEFINE_RE = new RegExp(String.raw`^(\s*)(define|defonce)\s*\(?\s*:${NAME}(?:.*?\bdo\s*(?:\|([^|]*)\|)?)?`, "u");
const LIVE_LOOP_RE = new RegExp(String.raw`\blive_loop\s*\(?\s*:${NAME}`, "gu");
const THREAD_RE = new RegExp(String.raw`\bin_thread\s*\(?\s*name:\s*:${NAME}`, "gu");
const CUE_RE = new RegExp(String.raw`\b(?:cue|set)\s*\(?\s*:${NAME}`, "gu");
const SYNTH_RE = /\b(?:use_synth|with_synth)\s*\(?\s*:([\p{L}\p{N}_]+)/gu;

function commentAbove(lines: string[], line: number): string | undefined {
  const out: string[] = [];
  for (let i = line - 1; i >= 0; i--) {
    const m = /^\s*#\s?(.*)$/.exec(lines[i]);
    if (!m) break;
    out.unshift(m[1]);
  }
  return out.length ? out.join("\n").trim() : undefined;
}

export function analyse(text: string): DocumentInfo {
  const lines = text.split(/\r?\n/);
  const definitions: Definition[] = [];
  const functions = new Map<string, Definition>();
  const cueNames = new Set<string>();
  let loadsSynthdefs = false;

  lines.forEach((full, line) => {
    const code = full.slice(0, codeEnd(full));
    if (/\bload_synthdefs?\b/.test(code)) loadsSynthdefs = true;

    const d = DEFINE_RE.exec(code);
    if (d) {
      const start = code.indexOf(`:${d[3]}`, d[1].length) + 1;
      const def: Definition = {
        name: d[3], kind: d[2] as DefinitionKind, line, start, end: start + d[3].length,
        params: d[4] ? d[4].split(",").map((p) => p.trim()).filter(Boolean) : [],
        doc: commentAbove(lines, line),
      };
      definitions.push(def);
      if (!functions.has(def.name)) functions.set(def.name, def);
    }
    for (const [re, kind] of [[LIVE_LOOP_RE, "live_loop"], [THREAD_RE, "thread"], [CUE_RE, "cue"]] as const) {
      for (const m of code.matchAll(re)) {
        const start = m.index + m[0].length - m[1].length;
        cueNames.add(m[1]);
        if (kind !== "cue") definitions.push({ name: m[1], kind, line, start, end: start + m[1].length, params: [], doc: commentAbove(lines, line) });
      }
    }
  });
  return { lines, definitions, functions, cueNames, loadsSynthdefs };
}

/** The synth `play` uses at a position: the last `use_synth`/`with_synth` above it, else `:beep`. */
export function currentSynthAt(lines: string[], line: number, col: number): string {
  for (let i = line; i >= 0; i--) {
    const l = i === line ? lines[i].slice(0, col) : lines[i];
    const code = l.slice(0, codeEnd(l));
    let last: string | undefined;
    for (const m of code.matchAll(SYNTH_RE)) last = m[1];
    if (last) return last;
  }
  return "beep";
}

const bare = (w: string | undefined): string => (w ?? "").replace(/^:/, "").replace(/^["']|["']$/g, "");

export interface OptOwner {
  /** How a hover names it: `the :prophet synth`, `sample`, `live_loop`. */
  label: string;
  kind: "synth" | "fx" | "sample" | "fn";
  /** True when the owner is named explicitly in the call (`synth :x`, `with_fx :y`), not inferred. */
  explicit: boolean;
  opts: Record<string, Opt>;
  /** The call that takes these opts, and its index among the context's words. */
  fn: string;
  index: number;
}

function fnOpts(fn: string): Record<string, Opt> {
  const out: Record<string, Opt> = {};
  for (const [k, doc] of Object.entries(own(data.functions, fn)?.opts ?? {})) out[k] = { doc };
  return out;
}

function withFnOpts(opts: Record<string, Opt>, fn: string): Record<string, Opt> {
  const out: Record<string, Opt> = { ...opts };
  for (const [k, o] of Object.entries(fnOpts(fn))) if (!(k in out)) out[k] = o;
  return out;
}

let anySynth: Record<string, Opt> | undefined;

/** Every opt any synth takes, documented, without bounds: they differ between synths. */
function anySynthOpts(): Record<string, Opt> {
  if (!anySynth) {
    anySynth = {};
    for (const synth of Object.values(data.synths)) {
      for (const [k, o] of Object.entries(synth.opts)) {
        if (!(k in anySynth)) anySynth[k] = { doc: o.doc, slidable: o.slidable, bpmScale: o.bpmScale };
      }
    }
  }
  return anySynth;
}

const OWNS_OPTS = (w: string): boolean =>
  w === "with_fx" || w === "synth" || w === "control" || PLAY_FNS.has(w) || SAMPLE_FNS.has(w) ||
  Object.keys(own(data.functions, w)?.opts ?? {}).length > 0;

/**
 * Whose opts a call's `key:` arguments are: the FX of a `with_fx`, the synth
 * of a `synth`, the current synth for `play`, the sampler for `sample`, or a
 * function's documented opts.
 */
export function optOwner(context: string[], lines: string[], line: number, col: number): OptOwner | undefined {
  const words = wordsBefore(context);
  const i = words.findIndex(OWNS_OPTS);
  if (i < 0) return undefined;
  const fn = words[i];
  const named = bare(words[i + 1]);
  const at = { fn, index: i };
  if (fn === "with_fx") {
    const fx = own(data.fx, named);
    if (fx) return { label: `the :${named} FX`, kind: "fx", explicit: true, opts: withFnOpts(fx.opts, fn), ...at };
    return { label: "with_fx", kind: "fn", explicit: false, opts: fnOpts(fn), ...at };
  }
  if (fn === "synth") {
    const synth = own(data.synths, named);
    if (synth) return { label: `the :${named} synth`, kind: "synth", explicit: true, opts: withFnOpts(synth.opts, "play"), ...at };
    return { label: "synth", kind: "fn", explicit: false, opts: fnOpts("play"), ...at };
  }
  if (PLAY_FNS.has(fn) || fn === "control") {
    const name = currentSynthAt(lines, line, col);
    const base = fn === "control" ? "play" : fn;
    const synth = own(data.synths, name);
    if (synth) return { label: `the :${name} synth`, kind: "synth", explicit: false, opts: withFnOpts(withFnOpts(synth.opts, base), "play"), ...at };
    // A synth this buffer doesn't know (a typo, or one it loads): any synth's opts, as `play`'s.
    return { label: base, kind: "fn", explicit: false, opts: withFnOpts(withFnOpts(anySynthOpts(), base), "play"), ...at };
  }
  if (SAMPLE_FNS.has(fn)) return { label: "sample", kind: "sample", explicit: true, opts: withFnOpts(data.sampleOpts, fn), ...at };
  return { label: fn, kind: "fn", explicit: true, opts: fnOpts(fn), ...at };
}

/** The opt itself, looked up for its owner at the cursor. */
export function lookupOpt(owner: OptOwner | undefined, key: string): Opt | undefined {
  return owner ? own(owner.opts, key.replace(/:$/, "")) : undefined;
}

/**
 * What kind of name belongs at the cursor's slot — `synth`, `fx`, `sample`,
 * `chord`, `scale`, `note`, `cue`, `tuning`, `random_source`, `track` — from
 * the called function's positional arg kinds, or from the opt whose value
 * it is.
 */
export function slotKind(context: string[]): string | undefined {
  const slot = callSlot(context, (w) => own(argKinds, w) !== undefined);
  const words = wordsBefore(context);
  const last = words[words.length - 1];
  if (last && isOptKey(last)) {
    if (last === "note:" || last.endsWith("_note:")) return "note";
    if (last === "sync:" || last === "sync_bpm:") return "cue";
    return undefined;
  }
  if (!slot || slot.inOpts) return undefined;
  return own(argKinds, slot.fn)?.[slot.argIndex] ?? undefined;
}
