// Signature help: the call the cursor is in, and which argument it is on.
import { MarkupKind, type SignatureHelp, type ParameterInformation } from "vscode-languageserver";
import { data, own } from "./data.js";
import { callSlot, lineToContext, logicalLine, scanLineToCaret } from "./context.js";
import type { DocumentInfo } from "./analysis.js";

export function signatureHelp(doc: DocumentInfo, line: number, col: number): SignatureHelp | null {
  const text = doc.lines[line];
  if (text === undefined) return null;
  const scan = scanLineToCaret(text, col);
  if (scan.inComment || scan.inString) return null;
  const logical = logicalLine(doc.lines, line, col);
  const context = lineToContext(logical.text, logical.col);
  const slot = callSlot(context, (w) => own(data.functions, w) !== undefined || doc.functions.has(w));
  if (!slot) return null;

  const fn = own(data.functions, slot.fn);
  const params: string[] = fn ? fn.args.map((a) => a.name) : doc.functions.get(slot.fn)!.params;
  const hasOpts = fn !== undefined && Object.keys(fn.opts).length > 0;
  const labels = hasOpts ? [...params, "opts…"] : params;

  let label = slot.fn + (labels.length ? " " : "");
  const parameters: ParameterInformation[] = labels.map((p, i) => {
    const start = label.length;
    label += p + (i < labels.length - 1 ? ", " : "");
    const arg = fn?.args[i];
    const documentation = arg ? arg.type.replace(/_/g, " ") : p === "opts…" ? Object.keys(fn!.opts).map((k) => `${k}:`).join(" ") : undefined;
    return { label: [start, start + p.length], documentation };
  });

  let active = slot.inOpts ? params.length : slot.argIndex;
  if (active >= labels.length) active = hasOpts ? params.length : labels.length - 1;
  const def = doc.functions.get(slot.fn);
  return {
    signatures: [{
      label,
      documentation: { kind: MarkupKind.Markdown, value: fn ? fn.summary : `User-defined function${def?.doc ? `\n\n${def.doc}` : ""}` },
      parameters,
    }],
    activeSignature: 0,
    activeParameter: Math.max(0, active),
  };
}
