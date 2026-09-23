import { writeFileSync } from "node:fs";

const [output, recorder, results] = process.argv.slice(2);
if (!output || !recorder || !results) {
  throw new Error("usage: make-hooks.mjs <output> <recorder> <results>");
}

const shellQuote = (value) => `'${value.replaceAll("'", `'"'"'`)}'`;
const command = `sh ${shellQuote(recorder)} ${shellQuote(results)}`;
const hooks = {};
for (const event of ["SessionStart", "SessionEnd", "UserPromptSubmit", "Stop"]) {
  hooks[event] = [{ hooks: [{ type: "command", command }] }];
}
writeFileSync(output, `${JSON.stringify({ hooks }, null, 2)}\n`);
