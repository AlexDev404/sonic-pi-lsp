import { test } from "node:test";
import assert from "node:assert/strict";
import {
  callSlot, caretAfterClosedValue, lineToContext, logicalLine, scanLineToCaret, tokenEndAtCaret, wordAt,
} from "../src/context.js";

// Cases from the native GUI's tests (app/gui-tests/completion_context.test.cpp).

test("scanLineToCaret tracks strings, comments and brackets", () => {
  assert.deepEqual(scanLineToCaret("play 60", 7), { inComment: false, inString: false, bracketDepth: 0 });
  assert.equal(scanLineToCaret('cue "/foo', 9).inString, true);
  assert.equal(scanLineToCaret("cue '/foo", 9).inString, true);
  assert.equal(scanLineToCaret('cue "/foo" ', 11).inString, false);
  assert.equal(scanLineToCaret('cue "a\\"b', 9).inString, true);
  assert.equal(scanLineToCaret("play 60 # note", 14).inComment, true);
  assert.equal(scanLineToCaret('puts "# not a comment', 21).inComment, false);
  assert.equal(scanLineToCaret("play (scale 60,", 15).bracketDepth, 1);
  assert.equal(scanLineToCaret("foo(bar)", 8).bracketDepth, 0);
  assert.equal(scanLineToCaret("a([{", 4).bracketDepth, 3);
});

test("tokenEndAtCaret extends to the end of the token", () => {
  assert.equal(tokenEndAtCaret("lpf: 70", 6), 7);
  assert.equal(tokenEndAtCaret("lpf: 70", 5), 7);
  assert.equal(tokenEndAtCaret("sample :ambi_choir", 11), 18);
  assert.equal(tokenEndAtCaret("pan: \n", 5), 5);
  assert.equal(tokenEndAtCaret("lpf: 70,pan", 7), 7);
});

test("lineToContext reduces to the innermost call", () => {
  assert.deepEqual(lineToContext("play 60, amp: 0.5, cut", 22), ["play", "60", "amp:", "0.5", "cut"]);
  assert.deepEqual(lineToContext("play (scale ", 12), ["scale", ""]);
  assert.deepEqual(lineToContext("play scale(60, ", 15), ["scale", "60", ""]);
  assert.deepEqual(lineToContext("sample :ambi_choir, pan: 0.5", 28).at(-1), "0.5");
  assert.deepEqual(lineToContext("puts 'a b', ", 12), ["puts", "'a b'", ""]);
});

test("lineToContext starts afresh after a statement modifier", () => {
  assert.deepEqual(lineToContext("live_loop :foo do sample :bd", 28), ["sample", ":bd"]);
  assert.deepEqual(lineToContext("x = 1; use_synth :sa", 20), ["use_synth", ":sa"]);
  assert.deepEqual(lineToContext("play 60 if one_in 2", 19), ["one_in", "2"]);
});

test("caretAfterClosedValue", () => {
  assert.ok(caretAfterClosedValue("play [60, 64]", 13));
  assert.ok(caretAfterClosedValue('play "foo"', 10));
  assert.ok(!caretAfterClosedValue("synth :sine, ", 13));
  assert.ok(!caretAfterClosedValue("play 60", 7));
});

test("wordAt classifies the token under the cursor", () => {
  assert.deepEqual(wordAt("use_synth :prophet", 2), { text: "use_synth", start: 0, end: 9, kind: "word" });
  assert.deepEqual(wordAt("use_synth :prophet", 10), { text: ":prophet", start: 10, end: 18, kind: "symbol" });
  assert.deepEqual(wordAt("use_synth :prophet", 18), { text: ":prophet", start: 10, end: 18, kind: "symbol" });
  assert.deepEqual(wordAt("play 60, cutoff: 80", 12), { text: "cutoff:", start: 9, end: 16, kind: "key" });
  assert.deepEqual(wordAt("play 60, cutoff: 80", 15), { text: "cutoff:", start: 9, end: 16, kind: "key" });
  assert.deepEqual(wordAt("sleep 0.25", 8), { text: "0.25", start: 6, end: 10, kind: "number" });
  assert.deepEqual(wordAt("pan: -1", 6), { text: "-1", start: 5, end: 7, kind: "number" });
  assert.deepEqual(wordAt("if factor?(4, 2)", 6), { text: "factor?", start: 3, end: 10, kind: "word" });
  assert.equal(wordAt('puts "play"', 7), undefined);
  assert.equal(wordAt("# play 60", 3), undefined);
});

test("logicalLine joins a call continued over several lines", () => {
  const lines = ["with_fx :reverb,", "  room: 0.8, # big", "  mi"];
  const l = logicalLine(lines, 2, 4);
  assert.deepEqual(lineToContext(l.text, l.col), ["with_fx", ":reverb", "room:", "0.8", "mi"]);
});

test("callSlot finds the call and argument index", () => {
  const isFn = (w: string) => ["play", "chord"].includes(w);
  assert.deepEqual(callSlot(lineToContext("chord :e3, ", 11), isFn), { fn: "chord", argIndex: 1, inOpts: false, optValueOf: undefined });
  assert.deepEqual(callSlot(lineToContext("play 60, amp: ", 14), isFn), { fn: "play", argIndex: 1, inOpts: true, optValueOf: "amp:" });
  assert.deepEqual(callSlot(lineToContext("play 60, amp: 1, ", 17), isFn), { fn: "play", argIndex: 1, inOpts: true, optValueOf: undefined });
});
