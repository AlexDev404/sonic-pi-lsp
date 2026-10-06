// Bundles the server, its dependencies and Sonic Pi's docs into one file,
// dist/sonic-pi-lsp.cjs, which runs with plain `node` anywhere.
import { build } from "esbuild";
import { readFileSync } from "node:fs";

const { version } = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));

await build({
  entryPoints: ["src/server.ts"],
  outfile: "dist/sonic-pi-lsp.cjs",
  bundle: true,
  platform: "node",
  target: "node18",
  format: "cjs",
  banner: { js: "#!/usr/bin/env node" },
  define: { SONIC_PI_LSP_VERSION: JSON.stringify(version) },
  legalComments: "none",
  logLevel: "warning",
});
