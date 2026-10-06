// Diagnostics: names Sonic Pi won't recognise, and opt values it will reject.
// Only what is certain from the source text is reported: literal names in
// slots that take a synth, FX, sample, chord or scale, and literal numbers
// given to opts with documented bounds.
import { DiagnosticSeverity, type Diagnostic } from "vscode-languageserver";
import { data, own, type Opt } from "./data.js";
import { codeEnd, lineToContext, logicalLine, scanLineToCaret } from "./context.js";
import { optOwner, slotKind, type DocumentInfo } from "./analysis.js";

const SOURCE = "sonic-pi";

const NAMED_KINDS: Record<string, { what: string; names: () => string[] }> = {
  synth: { what: "synth", names: () => Object.keys(data.synths) },
  fx: { what: "FX", names: () => Object.keys(data.fx) },
  sample: { what: "built-in sample", names: () => Object.keys(data.samples) },
  chord: { what: "chord", names: () => Object.keys(data.chords) },
  scale: { what: "scale", names: () => Object.keys(data.scales) },
  tuning: { what: "tuning", names: () => data.tunings },
  random_source: { what: "random source", names: () => data.randomSources },
};

function editDistance(a: string, b: string): number {
  const prev = Array.from({ length: b.length + 1 }, (_, j) => j);
  for (let i = 1; i <= a.length; i++) {
    let diag = prev[0];
    prev[0] = i;
    for (let j = 1; j <= b.length; j++) {
      const tmp = prev[j];
      prev[j] = Math.min(prev[j] + 1, prev[j - 1] + 1, diag + (a[i - 1] === b[j - 1] ? 0 : 1));
      diag = tmp;
    }
  }
  return prev[b.length];
}

export function closest(name: string, candidates: string[]): string | undefined {
  let best: string | undefined;
  let bestDist = Math.max(2, Math.floor(name.length / 3)) + 1;
  for (const c of candidates) {
    const d = editDistance(name, c);
    if (d < bestDist) { best = c; bestDist = d; }
  }
  return best;
}

/** Why Sonic Pi would reject `value` for this opt, else undefined. */
export function optViolation(opt: Opt, value: number): string | undefined {
  if (opt.options) {
    return opt.options.some((o) => Number(o) === value) ? undefined : `must be one of ${opt.options.join(", ")}`;
  }
  if (opt.min !== undefined && (opt.minIncl === false ? value <= opt.min : value < opt.min)) {
    return opt.minIncl === false ? `must be greater than ${opt.min}` : opt.min === 0 ? "must be zero or greater" : `must be at least ${opt.min}`;
  }
  if (opt.max !== undefined && (opt.maxIncl === false ? value >= opt.max : value > opt.max)) {
    return opt.maxIncl === false ? `must be less than ${opt.max}` : `must be at most ${opt.max}`;
  }
  return undefined;
}

const SYMBOL_RE = /(?<![:\p{L}\p{N}_]):([\p{L}_][\p{L}\p{N}_]*[?!]?)/gu;
const OPT_NUMBER_RE = /(?<![\p{L}\p{N}_:])([\p{L}_][\p{L}\p{N}_]*):\s*(-?\d+(?:\.\d+)?)(?![\p{L}\p{N}_.])/gu;

export function diagnostics(doc: DocumentInfo): Diagnostic[] {
  const out: Diagnostic[] = [];
  doc.lines.forEach((full, line) => {
    const code = full.slice(0, codeEnd(full));
    const contextAt = (col: number) => {
      const logical = logicalLine(doc.lines, line, col);
      return lineToContext(logical.text, logical.col);
    };
    const inString = (col: number) => scanLineToCaret(code, col).inString;

    for (const m of code.matchAll(SYMBOL_RE)) {
      const start = m.index;
      const end = start + m[0].length;
      if (inString(start)) continue;
      const kind = slotKind(contextAt(end));
      const named = kind && NAMED_KINDS[kind];
      if (!named) continue;
      if (kind === "synth" && doc.loadsSynthdefs) continue;
      const names = named.names();
      if (names.includes(m[1])) continue;
      const guess = closest(m[1], names);
      out.push({
        range: { start: { line, character: start }, end: { line, character: end } },
        severity: DiagnosticSeverity.Warning,
        source: SOURCE,
        message: `Unknown ${named.what} :${m[1]}${guess ? ` (did you mean :${guess}?)` : ""}`,
      });
    }

    for (const m of code.matchAll(OPT_NUMBER_RE)) {
      const start = m.index;
      const keyEnd = start + m[1].length + 1;
      if (inString(start)) continue;
      const owner = optOwner(contextAt(keyEnd), doc.lines, line, start);
      if (!owner) continue;
      const opt = own(owner.opts, m[1]);
      const end = start + m[0].length;
      const range = { start: { line, character: start }, end: { line, character: end } };
      if (!opt) {
        if (owner.explicit && (owner.kind === "synth" || owner.kind === "fx")) {
          const guess = closest(m[1], Object.keys(owner.opts));
          out.push({
            range: { start: { line, character: start }, end: { line, character: keyEnd } },
            severity: DiagnosticSeverity.Information,
            source: SOURCE,
            message: `${m[1]}: is not an opt of ${owner.label}, so Sonic Pi ignores it${guess ? ` (did you mean ${guess}:?)` : ""}`,
          });
        }
        continue;
      }
      const why = optViolation(opt, Number(m[2]));
      if (why) {
        out.push({
          range,
          severity: owner.explicit ? DiagnosticSeverity.Error : DiagnosticSeverity.Warning,
          source: SOURCE,
          message: `Value of opt :${m[1]} ${why}, got ${m[2]}.${owner.explicit ? "" : ` (checked against ${owner.label})`}`,
        });
      }
    }
  });
  return out;
}
