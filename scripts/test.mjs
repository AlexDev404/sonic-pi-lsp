// Runs the compiled tests, named file by file: `node --test <dir>` and
// `node --test <glob>` each work on only some of the Node versions we support.
import { readdirSync } from "node:fs";
import { spawnSync } from "node:child_process";

const dir = "build/test";
const files = readdirSync(dir).filter((f) => f.endsWith(".test.js")).map((f) => `${dir}/${f}`);
const { status } = spawnSync(process.execPath, ["--test", ...files], { stdio: "inherit" });
process.exit(status ?? 1);
