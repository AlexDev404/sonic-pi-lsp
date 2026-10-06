import { analyse, type DocumentInfo } from "../src/analysis.js";

/** A buffer with the cursor marked by `‸`: the analysed document and the cursor's line and column. */
export function at(source: string): { doc: DocumentInfo; line: number; col: number } {
  const lines = source.split("\n");
  const line = lines.findIndex((l) => l.includes("‸"));
  if (line < 0) throw new Error("no ‸ cursor in source");
  const col = lines[line].indexOf("‸");
  lines[line] = lines[line].slice(0, col) + lines[line].slice(col + 1);
  return { doc: analyse(lines.join("\n")), line, col };
}
