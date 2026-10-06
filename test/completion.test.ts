import { test } from "node:test";
import assert from "node:assert/strict";
import type { CompletionItem, TextEdit } from "vscode-languageserver";
import { completions, resolveCompletion } from "../src/completion.js";
import { at } from "./util.js";

const complete = (source: string) => {
  const { doc, line, col } = at(source);
  return completions(doc, line, col);
};
const labels = (items: CompletionItem[]) => items.map((i) => i.label);

test("a slot's kind decides the names offered", () => {
  assert.ok(labels(complete("use_synth ‸")).includes(":prophet"));
  assert.ok(labels(complete("with_fx :‸")).includes(":reverb"));
  assert.ok(labels(complete("sample :bd‸")).includes(":bd_haus"));
  assert.ok(labels(complete("play chord(:e3, :‸")).includes(":minor7"));
  assert.ok(labels(complete("play scale(:e3, ‸")).includes(":minor_pentatonic"));
  assert.ok(labels(complete("use_random_source ‸")).includes(":perlin"));
  assert.ok(labels(complete("play ‸")).includes(":c4"));
});

test("a symbol's completion replaces the partial including its colon", () => {
  const item = complete("use_synth :pro‸").find((i) => i.label === ":prophet")!;
  assert.deepEqual((item.textEdit as TextEdit).range, { start: { line: 0, character: 10 }, end: { line: 0, character: 14 } });
  assert.equal((item.textEdit as TextEdit).newText, ":prophet");
});

test("past a call's first argument come its opts", () => {
  const fx = labels(complete("with_fx :reverb, ‸"));
  assert.ok(fx.includes("room:") && fx.includes("mix:") && fx.includes("reps:"));
  assert.ok(labels(complete("synth :tb303, ‸")).includes("wave:"));
  assert.ok(labels(complete("sample :loop_amen, ‸")).includes("beat_stretch:"));
  assert.ok(labels(complete("live_loop :foo, ‸")).includes("sync:"));
  assert.ok(labels(complete("use_synth_defaults ‸")).includes("release:"));
});

test("play's opts follow use_synth", () => {
  const items = labels(complete("use_synth :tb303\nplay 60, ‸"));
  assert.ok(items.includes("wave:"));
  assert.ok(!labels(complete("play 60, ‸")).includes("wave:"));
});

test("opts already given are not offered again", () => {
  assert.ok(!labels(complete("with_fx :reverb, room: 0.5, ‸")).includes("room:"));
});

test("an opt's value: its options, or its default", () => {
  assert.deepEqual(labels(complete("synth :tb303, wave: ‸")), ["0", "1", "2"]);
  assert.deepEqual(labels(complete("with_fx :reverb, room: ‸")), ["0.6"]);
  assert.ok(labels(complete("synth :beep, note: ‸")).includes(":e3"));
});

test("cue names complete after sync", () => {
  const items = labels(complete("live_loop :drums do\n  cue :tick\nend\nsync ‸"));
  assert.deepEqual(items, [":drums", ":tick"]);
});

test("functions everywhere else, including the buffer's own", () => {
  const items = labels(complete("define :wobble do\nend\nwob‸"));
  assert.ok(items.includes("live_loop") && items.includes("wobble"));
});

test("nothing in comments or strings", () => {
  assert.deepEqual(complete("# use_synth ‸"), []);
  assert.deepEqual(complete('puts "use_synth ‸'), []);
});

test("resolve adds the documentation", () => {
  const item = complete("pla‸").find((i) => i.label === "play")!;
  assert.equal(item.documentation, undefined);
  const resolved = resolveCompletion(item);
  assert.match((resolved.documentation as { value: string }).value, /Play current synth/);
});
