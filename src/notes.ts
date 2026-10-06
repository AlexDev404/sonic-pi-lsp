// Note names as Sonic Pi reads them (app/server/ruby/lib/sonicpi/note.rb):
// a letter, an optional sharp (s) or flat (b, f), an optional octave
// (default 4), with C4 = MIDI 60 and A4 = 440 Hz.

const NOTE_RE = /^:?([a-gA-G])([sSbBfF]?)(-?[0-9]*)$/;
const LETTERS: Record<string, number> = { c: 0, d: 2, e: 4, f: 5, g: 7, a: 9, b: 11 };
const NAMES = ["C", "C#", "D", "Eb", "E", "F", "F#", "G", "Ab", "A", "Bb", "B"];
const SYMBOLS = ["c", "cs", "d", "eb", "e", "f", "fs", "g", "ab", "a", "bb", "b"];

/** The MIDI number of a note name like `:e3`, `:Cs4` or `eb`, else undefined. */
export function noteToMidi(name: string): number | undefined {
  const m = NOTE_RE.exec(name);
  if (!m) return undefined;
  const acc = m[2].toLowerCase();
  const interval = LETTERS[m[1].toLowerCase()] + (acc === "s" ? 1 : acc === "b" || acc === "f" ? -1 : 0);
  const octave = m[3] === "" || m[3] === "-" ? 4 : parseInt(m[3], 10);
  return (octave + 1) * 12 + interval;
}

export function midiToHz(midi: number): number {
  return 440 * Math.pow(2, (midi - 69) / 12);
}

/** `C4`, `Eb3`: the note's name, for a whole MIDI number. */
export function midiToName(midi: number): string {
  const n = Math.round(midi);
  return `${NAMES[((n % 12) + 12) % 12]}${Math.floor(n / 12) - 1}`;
}

/** `:c4`, `:eb3`: how the note is written in Sonic Pi. */
export function midiToSymbol(midi: number): string {
  const n = Math.round(midi);
  return `:${SYMBOLS[((n % 12) + 12) % 12]}${Math.floor(n / 12) - 1}`;
}

export function describeMidi(midi: number): string {
  const hz = midiToHz(midi);
  const whole = Number.isInteger(midi);
  return `${whole ? `${midiToName(midi)} · ` : ""}MIDI ${midi} · ${hz.toFixed(2)} Hz`;
}

/** Every note from octave 1 to 7, in each common spelling, for completion. */
export function noteSpellings(): { symbol: string; midi: number }[] {
  const spellings: [string, number][] = [
    ["c", 0], ["cs", 1], ["db", 1], ["d", 2], ["ds", 3], ["eb", 3], ["e", 4], ["f", 5],
    ["fs", 6], ["gb", 6], ["g", 7], ["gs", 8], ["ab", 8], ["a", 9], ["as", 10], ["bb", 10], ["b", 11],
  ];
  const out: { symbol: string; midi: number }[] = [];
  for (let octave = 1; octave <= 7; octave++) {
    for (const [name, offset] of spellings) out.push({ symbol: `:${name}${octave}`, midi: (octave + 1) * 12 + offset });
  }
  return out;
}
