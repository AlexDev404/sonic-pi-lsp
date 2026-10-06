import { test } from "node:test";
import assert from "node:assert/strict";
import { hover } from "../src/hover.js";
import { at } from "./util.js";

const hoverAt = (source: string) => {
  const { doc, line, col } = at(source);
  return hover(doc, line, col);
};

test("a function shows its usage, summary and doc", () => {
  const h = hoverAt("pl‸ay 60");
  assert.ok(h);
  assert.match(h.markdown, /^```ruby\nplay /);
  assert.match(h.markdown, /\*\*Play current synth\*\*/);
  assert.deepEqual([h.start, h.end], [0, 4]);
});

test("a synth, an FX and a sample", () => {
  assert.match(hoverAt("use_synth :pro‸phet")!.markdown, /\*\*The Prophet\*\* \(synth\)/);
  assert.match(hoverAt("with_fx :rev‸erb do\nend")!.markdown, /\*\*Reverb\*\* \(FX\)/);
  assert.match(hoverAt("sample :bd_h‸aus")!.markdown, /built-in sample[\s\S]*Bass Drums/);
});

test("an opt is described as its owner's", () => {
  assert.match(hoverAt("with_fx :reverb, ro‸om: 0.8 do\nend")!.markdown, /opt of the :reverb FX[\s\S]*between 0 and 1/);
  assert.match(hoverAt("synth :tb303, cut‸off: 80")!.markdown, /opt of the :tb303 synth/);
  assert.match(hoverAt("sample :loop_amen, ra‸te: 0.5")!.markdown, /opt of sample/);
});

test("play's opts are the current synth's", () => {
  const h = hoverAt("use_synth :prophet\nplay 60, res‸: 0.5");
  assert.match(h!.markdown, /opt of the :prophet synth/);
  assert.match(hoverAt("play 60, rel‸ease: 2")!.markdown, /opt of the :beep synth/);
});

test("an opt continued onto the next line keeps its owner", () => {
  assert.match(hoverAt("with_fx :echo,\n  pha‸se: 0.25 do\nend")!.markdown, /opt of the :echo FX/);
});

test("notes: names and numbers in a note slot", () => {
  assert.match(hoverAt("play :e‸3")!.markdown, /E3 · MIDI 52 · 164\.81 Hz/);
  assert.match(hoverAt("play 6‸9")!.markdown, /A4 · MIDI 69 · 440\.00 Hz/);
  assert.equal(hoverAt("sleep 6‸9"), undefined);
});

test("chords and scales list their notes from the given root", () => {
  assert.match(hoverAt("play chord(:e3, :min‸or)")!.markdown, /chord[\s\S]*From :e3: :e3, :g3, :b3/);
  assert.match(hoverAt("play_pattern scale(:c4, :major_pent‸atonic)")!.markdown, /scale/);
});

test("user definitions: define with its comment, and live loops a sync waits on", () => {
  const src = "# Plays a kick.\ndefine :kick do |amp|\n  sample :bd_haus, amp: amp\nend\nki‸ck 1";
  const h = hoverAt(src);
  assert.match(h!.markdown, /kick amp/);
  assert.match(h!.markdown, /Plays a kick\./);
  assert.match(hoverAt("live_loop :beat do\n  sleep 1\nend\nsync :be‸at")!.markdown, /Live loop, line 1/);
});

test("nothing in comments, strings, or for unknown words", () => {
  assert.equal(hoverAt("# pl‸ay"), undefined);
  assert.equal(hoverAt('puts "pl‸ay"'), undefined);
  assert.equal(hoverAt("fo‸obar = 1"), undefined);
});
