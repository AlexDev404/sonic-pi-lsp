import { test } from "node:test";
import assert from "node:assert/strict";
import { DiagnosticSeverity } from "vscode-languageserver";
import { analyse } from "../src/analysis.js";
import { diagnostics } from "../src/diagnostics.js";
import { data } from "../src/data.js";

const check = (source: string) => diagnostics(analyse(source)).map((d) => ({ line: d.range.start.line, severity: d.severity, message: d.message as string }));

test("unknown names in named slots, with a suggestion", () => {
  assert.deepEqual(check("use_synth :prohpet"), [
    { line: 0, severity: DiagnosticSeverity.Warning, message: "Unknown synth :prohpet (did you mean :prophet?)" },
  ]);
  assert.match(check("with_fx :revrb do\nend")[0].message, /Unknown FX :revrb \(did you mean :reverb\?\)/);
  assert.match(check("sample :bd_hause")[0].message, /Unknown built-in sample :bd_hause/);
  assert.match(check("play chord(:e3, :mnor)")[0].message, /Unknown chord :mnor/);
});

test("custom synths are not flagged when the buffer loads synthdefs", () => {
  assert.deepEqual(check('load_synthdefs "~/synths"\nuse_synth :my_synth'), []);
});

test("opt values out of their bounds", () => {
  assert.deepEqual(check("with_fx :reverb, room: 2 do\nend"), [
    { line: 0, severity: DiagnosticSeverity.Error, message: "Value of opt :room must be at most 1, got 2." },
  ]);
  assert.match(check("synth :prophet, cutoff: 131")[0].message, /must be less than 131, got 131/);
  assert.match(check("synth :tb303, wave: 3")[0].message, /must be one of 0, 1, 2, got 3/);
  assert.match(check("sample :bd_haus, amp: -1")[0].message, /must be zero or greater, got -1/);
});

test("play's checks are against the current synth, as a warning", () => {
  const [d] = check("use_synth :prophet\nplay 60, cutoff: 200");
  assert.equal(d.severity, DiagnosticSeverity.Warning);
  assert.match(d.message, /checked against the :prophet synth/);
});

test("opts a synth or FX does not take are noted, not warned", () => {
  const [d] = check("with_fx :echo, room: 0.5 do\nend");
  assert.equal(d.severity, DiagnosticSeverity.Information);
  assert.match(d.message, /room: is not an opt of the :echo FX, so Sonic Pi ignores it/);
});

test("slide opts are opts", () => {
  assert.deepEqual(check("synth :prophet, cutoff: 70, cutoff_slide: 8"), []);
});

test("a call continued over lines is checked as one", () => {
  assert.match(check("with_fx :reverb,\n  room: 3 do\nend")[0].message, /Value of opt :room/);
});

test("valid code, comments and strings are clean", () => {
  assert.deepEqual(check('use_synth :prophet # :nope\nputs "use_synth :nope"\nplay 60, cutoff: 100, release: 2'), []);
});

test("Sonic Pi's own doc examples raise no errors or warnings", () => {
  const problems: string[] = [];
  for (const [name, fn] of Object.entries(data.functions)) {
    fn.examples.forEach((example) => {
      for (const d of diagnostics(analyse(example))) {
        if (d.severity === DiagnosticSeverity.Information) continue;
        // with_synth's example names a made-up synth on purpose.
        if (name === "with_synth" && (d.message as string).includes(":saw_beep")) continue;
        problems.push(`${name}: ${d.message}`);
      }
    });
  }
  assert.deepEqual(problems, []);
});
