import { test } from "node:test";
import assert from "node:assert/strict";
import { signatureHelp } from "../src/signature.js";
import { at } from "./util.js";

const sig = (source: string) => {
  const { doc, line, col } = at(source);
  return signatureHelp(doc, line, col);
};

test("the active argument moves through positionals into opts", () => {
  const first = sig("chord ‸")!;
  assert.equal(first.signatures[0].label, "chord tonic, name, opts…");
  assert.equal(first.activeParameter, 0);
  assert.equal(sig("chord :e3, ‸")!.activeParameter, 1);
  assert.equal(sig("chord :e3, :minor, invert: ‸")!.activeParameter, 2);
});

test("the innermost call wins", () => {
  assert.equal(sig("play chord(:e3, ‸")!.signatures[0].label.split(" ")[0], "chord");
});

test("user-defined functions take their block parameters", () => {
  const s = sig("define :hit do |n, amp|\nend\nhit 60, ‸")!;
  assert.equal(s.signatures[0].label, "hit n, amp");
  assert.equal(s.activeParameter, 1);
});

test("none outside a call", () => {
  assert.equal(sig("‸"), null);
  assert.equal(sig("# play ‸"), null);
});
