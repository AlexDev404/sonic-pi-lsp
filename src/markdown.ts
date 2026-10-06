// Markdown for hovers and completion details. Plain CommonMark (headings,
// fenced code, lists, inline code) so every LSP client renders it.
import { data, type Fn, type Instrument, type Opt, type Value } from "./data.js";
import type { Definition } from "./analysis.js";
import { describeMidi, midiToSymbol } from "./notes.js";

const fence = (code: string): string => "```ruby\n" + code.trim() + "\n```";

function fmt(v: Value): string {
  return typeof v === "number" && !Number.isInteger(v) ? String(Number(v.toFixed(4))) : String(v);
}

export function fnUsage(name: string, fn: Fn): string {
  const args = fn.args.map((a) => a.name);
  const opts = Object.keys(fn.opts).length ? ["opts…"] : [];
  return [name, [...args, ...opts].join(", ")].filter(Boolean).join(" ");
}

export interface DocOptions {
  /** Include every example, not just the first. */
  allExamples?: boolean;
}

export function functionMarkdown(name: string, fn: Fn, options: DocOptions = {}): string {
  const parts: string[] = [fence(fn.usage ?? fnUsage(name, fn)), `**${fn.summary}**`];
  if (fn.args.length) parts.push(fn.args.map((a) => `- \`${a.name}\` — ${a.type.replace(/_/g, " ")}`).join("\n"));
  if (fn.doc) parts.push(fn.doc);
  const opts = Object.entries(fn.opts);
  if (opts.length) parts.push("**Opts**\n\n" + opts.map(([k, d]) => `- \`${k}:\` ${firstSentence(d)}`).join("\n"));
  const examples = options.allExamples ? fn.examples : fn.examples.slice(0, 1);
  if (examples.length) parts.push(`**Example${examples.length > 1 ? "s" : ""}**\n\n` + examples.map(fence).join("\n\n"));
  const meta = [fn.introduced && `Introduced in v${fn.introduced}`, fn.acceptsBlock && (fn.requiresBlock ? "Requires a `do … end` block" : "Accepts a `do … end` block")].filter(Boolean);
  if (meta.length) parts.push(`*${meta.join(" · ")}*`);
  return parts.join("\n\n");
}

function firstSentence(s: string): string {
  const t = s.replace(/\s+/g, " ").trim();
  return /^.+?[.!?](?=\s|$)/.exec(t)?.[0] ?? t;
}

export function constraintText(opt: Opt): string | undefined {
  if (opt.constraints?.length) return opt.constraints.join("; ");
  if (opt.options) return `must be one of ${opt.options.map(fmt).join(", ")}`;
  return undefined;
}

export function optMarkdown(key: string, opt: Opt, ownerLabel?: string): string {
  const name = key.replace(/:$/, "");
  const parts = [`**\`${name}:\`**${ownerLabel ? ` — opt of ${ownerLabel}` : ""}`];
  if (opt.doc) parts.push(opt.doc);
  const facts: string[] = [];
  if (opt.default !== undefined) {
    const note = typeof opt.default === "number" && opt.midi ? ` (${describeMidi(opt.default)})` : "";
    facts.push(`Default: \`${fmt(opt.default)}\`${note}`);
  }
  const cons = constraintText(opt);
  if (cons) facts.push(cons.charAt(0).toUpperCase() + cons.slice(1));
  if (opt.bpmScale) facts.push("Measured in beats (scaled by the current BPM)");
  if (opt.slidable) facts.push(`Slidable: set \`${name}_slide:\` to glide to new values with \`control\``);
  if (facts.length) parts.push(facts.map((f) => `- ${f}`).join("\n"));
  return parts.join("\n\n");
}

const SLIDE_SUFFIX = /_slide(_shape|_curve)?$/;

export function instrumentMarkdown(name: string, inst: Instrument, kind: "synth" | "fx"): string {
  const usage = kind === "synth" ? `use_synth :${name}` : `with_fx :${name} do\n  # ...\nend`;
  const parts = [fence(usage), `**${inst.title}** (${kind === "synth" ? "synth" : "FX"})`, inst.doc];
  const rows = Object.entries(inst.opts)
    .filter(([k]) => !SLIDE_SUFFIX.test(k))
    .map(([k, o]) => `| \`${k}:\` | ${o.default !== undefined ? `\`${fmt(o.default)}\`` : ""} | ${firstSentence(o.doc).replace(/\|/g, "\\|")} |`);
  if (rows.length) {
    parts.push("**Opts**\n\n| Opt | Default | |\n|---|---|---|\n" + rows.join("\n"));
    if (Object.keys(inst.opts).some((k) => SLIDE_SUFFIX.test(k))) parts.push("*Slidable opts also take `_slide:`, `_slide_shape:` and `_slide_curve:` variants.*");
  }
  return parts.join("\n\n");
}

export function sampleMarkdown(name: string): string {
  const group = data.samples[name];
  return [fence(`sample :${name}`), `**:${name}** — built-in sample`, group ? `One of the *${group}*.` : ""].filter(Boolean).join("\n\n");
}

function intervalsMarkdown(kind: "chord" | "scale", name: string, intervals: number[], tonic?: number): string {
  const root = tonic ?? 60;
  const notes = intervals.map((i) => midiToSymbol(root + i)).join(", ");
  const usage = kind === "chord" ? `chord ${tonic !== undefined ? midiToSymbol(root) : ":c4"}, :${name}` : `scale ${tonic !== undefined ? midiToSymbol(root) : ":c4"}, :${name}`;
  return [
    fence(usage),
    `**:${name}** — ${kind}`,
    `Semitones from the root: \`${intervals.join(", ")}\``,
    `From ${midiToSymbol(root)}: ${notes}`,
  ].join("\n\n");
}

export function chordMarkdown(name: string, intervals: number[], tonic?: number): string {
  return intervalsMarkdown("chord", name, intervals, tonic);
}

export function scaleMarkdown(name: string, intervals: number[], tonic?: number): string {
  return intervalsMarkdown("scale", name, intervals, tonic);
}

export function noteMarkdown(text: string, midi: number): string {
  return `**${text}** — note\n\n${describeMidi(midi)}`;
}

export function definitionMarkdown(def: Definition): string {
  const parts: string[] = [];
  if (def.kind === "define" || def.kind === "defonce") {
    parts.push(fence(`${def.name}${def.params.length ? " " + def.params.join(", ") : ""}`));
    parts.push(`User-defined function (\`${def.kind} :${def.name}\`, line ${def.line + 1})`);
  } else if (def.kind === "live_loop") {
    parts.push(fence(`live_loop :${def.name}`), `Live loop, line ${def.line + 1}. Each iteration sends the cue \`/live_loop/${def.name}\`, so \`sync :${def.name}\` waits for its next iteration.`);
  } else if (def.kind === "thread") {
    parts.push(fence(`in_thread name: :${def.name}`), `Named thread, line ${def.line + 1}.`);
  }
  if (def.doc) parts.push(def.doc);
  return parts.join("\n\n");
}
