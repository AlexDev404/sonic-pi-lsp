// Sonic Pi's language facts, as tools/extract-docs.rb extracted them from
// Sonic Pi's own doc system (data/sonic-pi.json).
import raw from "../data/sonic-pi.json" with { type: "json" };

export type Value = number | string | boolean;

/** A synth, FX or function opt, with the bounds Sonic Pi validates it against. */
export interface Opt {
  doc: string;
  default?: Value;
  options?: Value[];
  min?: number;
  minIncl?: boolean;
  max?: number;
  maxIncl?: boolean;
  constraints?: string[];
  slidable?: boolean;
  bpmScale?: boolean;
  midi?: boolean;
}

export interface Fn {
  summary: string;
  args: { name: string; type: string }[];
  /** Documented opts: name (no colon) to doc. */
  opts: Record<string, string>;
  doc: string;
  examples: string[];
  usage?: string;
  introduced?: string;
  acceptsBlock?: boolean;
  requiresBlock?: boolean;
  returns?: string;
  /** What each positional arg names (synth, fx, sample, cue, ...), where tagged. */
  argKinds?: (string | null)[];
}

export interface Instrument {
  title: string;
  summary: string;
  doc: string;
  opts: Record<string, Opt>;
}

export interface SonicPiData {
  sonicPiVersion: string;
  functions: Record<string, Fn>;
  synths: Record<string, Instrument>;
  fx: Record<string, Instrument>;
  sampleOpts: Record<string, Opt>;
  /** Built-in sample name to its group's title. */
  samples: Record<string, string>;
  sampleGroups: { title: string; samples: string[] }[];
  /** Chord / scale name to semitone offsets from the root. */
  chords: Record<string, number[]>;
  scales: Record<string, number[]>;
  tunings: string[];
  randomSources: string[];
}

export const data = raw as unknown as SonicPiData;

/** Own-property lookup, so names like `constructor` never hit Object.prototype. */
export function own<T>(table: Record<string, T>, key: string): T | undefined {
  return Object.prototype.hasOwnProperty.call(table, key) ? table[key] : undefined;
}

/**
 * The kind of value each positional argument takes, per function: the doc
 * system's `arg_kinds:` tags, plus the slots the native GUI recognises by name
 * (note, tuning and random-source slots, and chord/scale tonics).
 */
export const argKinds: Record<string, (string | null)[]> = (() => {
  const table: Record<string, (string | null)[]> = {};
  for (const [name, fn] of Object.entries(data.functions)) {
    if (fn.argKinds) table[name] = fn.argKinds;
  }
  const extra: Record<string, (string | null)[]> = {
    play: ["note"],
    note: ["note"],
    note_info: ["note"],
    midi: ["note"],
    midi_note_on: ["note"],
    midi_note_off: ["note"],
    chord: ["note", "chord"],
    scale: ["note", "scale"],
    chord_degree: [null, "note", "scale"],
    degree: [null, "note", "scale"],
    use_random_source: ["random_source"],
    with_random_source: ["random_source"],
    use_tuning: ["tuning"],
    with_tuning: ["tuning"],
  };
  for (const [name, kinds] of Object.entries(extra)) {
    if (own(data.functions, name)) table[name] = kinds;
  }
  return table;
})();

/** The functions whose opts are the current synth's (set by `use_synth`). */
export const PLAY_FNS = new Set([
  "play", "play_chord", "play_pattern", "play_pattern_timed",
  "use_synth_defaults", "with_synth_defaults", "use_merged_synth_defaults", "with_merged_synth_defaults",
]);

/** The functions whose opts are the sampler's. */
export const SAMPLE_FNS = new Set([
  "sample", "use_sample_defaults", "with_sample_defaults", "use_merged_sample_defaults", "with_merged_sample_defaults",
]);
