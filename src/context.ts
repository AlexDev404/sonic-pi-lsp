// Where the cursor sits in Sonic Pi code: inside a string or comment, which
// call it is an argument of, and which argument. Ported from the native GUI's
// completion engine (app/gui/utils/completion_context.cpp), so the server reads
// code the way Sonic Pi's own editor does.

export interface LineScan {
  inComment: boolean;
  inString: boolean;
  bracketDepth: number;
}

/** Scan `line` over [0, col), Ruby-style: `\` escapes in strings, `#` outside one starts a comment. */
export function scanLineToCaret(line: string, col: number): LineScan {
  const s: LineScan = { inComment: false, inString: false, bracketDepth: 0 };
  let quote = "";
  const end = Math.max(0, Math.min(col, line.length));
  for (let i = 0; i < end; i++) {
    const c = line[i];
    if (s.inString) {
      if (c === "\\") { i++; continue; }
      if (c === quote) { s.inString = false; quote = ""; }
      continue;
    }
    if (c === "#") { s.inComment = true; break; }
    if (c === '"' || c === "'") { s.inString = true; quote = c; continue; }
    if (c === "(" || c === "[" || c === "{") { s.bracketDepth++; continue; }
    if ((c === ")" || c === "]" || c === "}") && s.bracketDepth > 0) s.bracketDepth--;
  }
  return s;
}

/** The column where the line's comment starts, or its length when it has none. */
export function codeEnd(line: string): number {
  let quote = "";
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (quote) {
      if (c === "\\") { i++; continue; }
      if (c === quote) quote = "";
      continue;
    }
    if (c === "#") return i;
    if (c === '"' || c === "'") quote = c;
  }
  return line.length;
}

function isTokenSeparator(c: string): boolean {
  return " \t\n\r,(){}[]\"'#".includes(c);
}

/** Extend a column to the end of the token it sits in, so `lpf: 7|0` sees `70`. */
export function tokenEndAtCaret(line: string, col: number): number {
  let end = Math.max(0, Math.min(col, line.length));
  while (end < line.length && !isTokenSeparator(line[end])) end++;
  return end;
}

const isWordChar = (c: string | undefined): boolean => c !== undefined && /[\p{L}\p{N}_]/u.test(c);
const STATEMENT_MODIFIERS = new Set(["if", "unless", "while", "until", "and", "or", "then", "do"]);

/**
 * Reduce a line, up to `col`, to the tokens of the innermost call the cursor
 * is in. The last element is the partial token at the cursor; the first is
 * usually the function being called. `play 60, amp: 0.5, cut|` gives
 * `["play", "60", "amp:", "0.5", "cut"]`.
 */
export function lineToContext(fullLine: string, col: number): string[] {
  let line = fullLine.slice(0, tokenEndAtCaret(fullLine, col));

  // A statement modifier or operator (`if`, `do`, `;`, `&&` ...) ends a call's
  // arguments: what follows is a fresh expression.
  {
    let depth = 0;
    let cut = -1;
    let quote = "";
    for (let i = 0; i < line.length; i++) {
      const c = line[i];
      if (quote) { if (c === quote) quote = ""; continue; }
      if (c === '"' || c === "'") { quote = c; continue; }
      if (c === "(" || c === "[" || c === "{") { depth++; continue; }
      if (c === ")" || c === "]" || c === "}") { if (depth > 0) depth--; continue; }
      if (depth !== 0) continue;
      if (c === ";") { cut = i + 1; continue; }
      if ((c === "&" && line[i + 1] === "&") || (c === "|" && line[i + 1] === "|")) { cut = i + 2; i++; continue; }
      const startsWord = /[\p{L}_]/u.test(c) && !isWordChar(line[i - 1]);
      if (!startsWord) continue;
      let j = i;
      while (j < line.length && isWordChar(line[j])) j++;
      if (STATEMENT_MODIFIERS.has(line.slice(i, j))) cut = j;
      i = j - 1;
    }
    if (cut >= 0) {
      while (cut < line.length && line[cut] === " ") cut++;
      line = line.slice(cut);
    }
  }

  // Nested calls resolve to the innermost: `play (scale ` is scale's.
  const open: number[] = [];
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (c === "(" || c === "[" || c === "{") open.push(i);
    else if ((c === ")" || c === "]" || c === "}") && open.length) open.pop();
  }
  if (open.length) {
    // Keep the function name when '(' is its call paren (`scale(60,`); a space before '(' marks grouping.
    const innermost = open[open.length - 1];
    let s = innermost;
    while (s > 0 && isWordChar(line[s - 1])) s--;
    const fn = line.slice(s, innermost);
    line = line.slice(innermost + 1);
    if (fn) line = `${fn} ${line}`;
  }

  // Split on spaces, commas and brackets, keeping a string as one token.
  const out: string[] = [];
  let cur = "";
  let quote = "";
  let inSeparators = false;
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (quote) {
      cur += c;
      if (c === "\\" && i + 1 < line.length) { cur += line[++i]; continue; }
      if (c === quote) quote = "";
      continue;
    }
    if (" ,(){}\t".includes(c)) {
      if (!inSeparators) { out.push(cur); cur = ""; inSeparators = true; }
      continue;
    }
    inSeparators = false;
    if (c === '"' || c === "'") quote = c;
    cur += c;
  }
  out.push(cur);
  return out;
}

export const isOptKey = (w: string): boolean => w.length > 1 && w.endsWith(":") && !w.startsWith(":");

/** The non-empty tokens before the partial. */
export function wordsBefore(context: string[]): string[] {
  return context.slice(0, -1).filter((w) => w !== "");
}

export interface CallSlot {
  fn: string;
  /** Index of the positional argument at the cursor. */
  argIndex: number;
  /** True once an opt key (`amp:`) has appeared in the call. */
  inOpts: boolean;
  /** The opt key whose value is at the cursor, e.g. `amp:`. */
  optValueOf?: string;
}

/**
 * The call the cursor is an argument of: the nearest preceding token that
 * `isFn` accepts, the positional index between it and the cursor, and whether
 * the call has reached its opts.
 */
export function callSlot(context: string[], isFn: (w: string) => boolean): CallSlot | undefined {
  const words = wordsBefore(context);
  const last = words[words.length - 1];
  const optValueOf = last !== undefined && isOptKey(last) ? last : undefined;
  let argIndex = 0;
  let inOpts = false;
  for (let i = words.length - 1; i >= 0; i--) {
    const w = words[i];
    if (isFn(w)) return { fn: w, argIndex, inOpts, optValueOf };
    if (isOptKey(w)) { inOpts = true; continue; }
    if (i > 0 && isOptKey(words[i - 1])) continue; // an opt's value is not a positional argument
    argIndex++;
  }
  return undefined;
}

/** True when the cursor directly follows a closed value (`)`, `]`, a quote), where nothing completes. */
export function caretAfterClosedValue(line: string, col: number): boolean {
  const prev = line[Math.min(col, line.length) - 1];
  return prev === ")" || prev === "]" || prev === "}" || prev === '"' || prev === "'";
}

/**
 * The statement the cursor's line continues: Sonic Pi calls often wrap their
 * opts onto following lines after a trailing comma. Joins the line with the
 * lines it continues and maps `col` into the joined text.
 */
export function logicalLine(lines: string[], lineNo: number, col: number): { text: string; col: number } {
  const continues = (l: string): boolean => /(,|\\|\(|\[)\s*$/.test(l.slice(0, codeEnd(l)));
  let start = lineNo;
  while (start > 0 && lineNo - start < 20 && continues(lines[start - 1])) start--;
  let text = "";
  for (let i = start; i < lineNo; i++) {
    const l = lines[i];
    text += l.slice(0, codeEnd(l)).replace(/\\\s*$/, "").trim() + " ";
  }
  return { text: text + lines[lineNo], col: text.length + col };
}

export type WordKind = "symbol" | "key" | "word" | "number";

export interface Word {
  text: string;
  start: number;
  end: number;
  kind: WordKind;
}

/**
 * The token under the cursor: a `:symbol`, an opt `key:`, a number, or a
 * plain word (a function or variable, with any trailing `?` or `!`).
 * Undefined inside strings and comments.
 */
export function wordAt(line: string, col: number): Word | undefined {
  let s = Math.min(col, line.length);
  let e = s;
  // On a symbol's leading colon, or just past a key's trailing colon: step onto the word.
  if (!isWordChar(line[s]) && line[s] === ":" && isWordChar(line[s + 1])) { s++; e = s; }
  else if (!isWordChar(line[s]) && !isWordChar(line[s - 1]) && line[s - 1] === ":" && isWordChar(line[s - 2])) { s -= 2; e = s + 1; }
  else if (!isWordChar(line[s]) && line[s - 1] !== undefined && isWordChar(line[s - 1])) { s--; e = s; }
  if (!isWordChar(line[s])) return undefined;
  while (s > 0 && isWordChar(line[s - 1])) s--;
  while (e < line.length && isWordChar(line[e])) e++;
  // A number with a fraction (`0.25`) is one token.
  if (/^\d+$/.test(line.slice(s, e)) && line[e] === "." && /\d/.test(line[e + 1] ?? "")) {
    e++;
    while (e < line.length && /\d/.test(line[e])) e++;
  }
  if (line[s - 1] === "." && /\d/.test(line[s - 2] ?? "") && /^\d+$/.test(line.slice(s, e))) {
    s--;
    while (s > 0 && /\d/.test(line[s - 1])) s--;
  }
  if (line[s - 1] === "-" && /^[\d.]+$/.test(line.slice(s, e)) && !isWordChar(line[s - 2])) s--;

  const scan = scanLineToCaret(line, s);
  if (scan.inComment || scan.inString) return undefined;

  const text = line.slice(s, e);
  if (/^-?[\d.]+$/.test(text)) return { text, start: s, end: e, kind: "number" };
  if (line[s - 1] === ":" && line[s - 2] !== ":") {
    let end = e;
    if (line[end] === "?" || line[end] === "!") end++;
    return { text: line.slice(s - 1, end), start: s - 1, end, kind: "symbol" };
  }
  if (line[e] === ":" && line[e + 1] !== ":") return { text: `${text}:`, start: s, end: e + 1, kind: "key" };
  if ((line[e] === "?" || line[e] === "!") && line[e + 1] !== "=") return { text: `${text}${line[e]}`, start: s, end: e + 1, kind: "word" };
  return { text, start: s, end: e, kind: "word" };
}
