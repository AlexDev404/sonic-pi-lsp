// Copies the bundled server into the extension before packaging.
// Build it first from the repository root: npm run build
import { copyFileSync, existsSync, mkdirSync } from "node:fs";

const server = new URL("../../dist/sonic-pi-lsp.cjs", import.meta.url);
if (!existsSync(server)) {
  console.error("dist/sonic-pi-lsp.cjs is missing: run `npm run build` in the repository root first.");
  process.exit(1);
}
mkdirSync(new URL("./server/", import.meta.url), { recursive: true });
copyFileSync(server, new URL("./server/sonic-pi-lsp.cjs", import.meta.url));
