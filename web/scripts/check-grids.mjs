// Fails when a responsive grid (sm:/md:/lg:/xl:grid-cols-*) has no base grid-cols-*.
// Without one, on a phone the grid's single column grows to its widest content (e.g. a long
// line meant to be cut off with "…"), the page gets wider than the screen and Safari shrinks it.
// Fix: add grid-cols-1, which caps the column at the screen width.
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

const roots = ["app", "components"];
const files = [];
const walk = (dir) => {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p);
    else if (p.endsWith(".tsx")) files.push(p);
  }
};
roots.forEach(walk);

const problems = [];
for (const f of files) {
  readFileSync(f, "utf8")
    .split("\n")
    .forEach((line, i) => {
      for (const m of line.matchAll(/className=(?:"|\{cn\(")([^"]*)"/g)) {
        const toks = m[1].split(/\s+/);
        const responsive = toks.some((t) => /^(sm|md|lg|xl|2xl):grid-cols-/.test(t));
        if (toks.includes("grid") && responsive && !toks.some((t) => t.startsWith("grid-cols-"))) {
          problems.push(`${f}:${i + 1}: grid without a phone column rule (add grid-cols-1): ${m[1]}`);
        }
      }
    });
}
if (problems.length) {
  console.error(problems.join("\n"));
  process.exit(1);
}
console.log(`✔ Responsive grids all have a phone column rule (${files.length} files checked)`);
