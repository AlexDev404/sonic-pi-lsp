// Readies the extension for packaging: bundles the client (extension.js and
// vscode-languageclient) into out/extension.js, and copies in the bundled
// server. Build the server first from the repository root: npm run build
import { copyFileSync, existsSync, mkdirSync } from "node:fs";
import { build } from "esbuild";

const server = new URL("../../dist/sonic-pi-lsp.cjs", import.meta.url);
if (!existsSync(server)) {
  console.error("dist/sonic-pi-lsp.cjs is missing: run `npm run build` in the repository root first.");
  process.exit(1);
}
mkdirSync(new URL("./server/", import.meta.url), { recursive: true });
copyFileSync(server, new URL("./server/sonic-pi-lsp.cjs", import.meta.url));

await build({
  entryPoints: ["extension.js"],
  outfile: "out/extension.js",
  bundle: true,
  platform: "node",
  format: "cjs",
  target: "node20",
  external: ["vscode"],
  minify: true,
  legalComments: "none",
  logLevel: "warning",
});
