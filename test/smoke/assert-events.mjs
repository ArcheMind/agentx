import { readFileSync } from "node:fs";

const [path, provider] = process.argv.slice(2);
let content;
try {
  content = readFileSync(path, "utf8");
} catch (error) {
  console.error(`${provider} did not emit any portable hooks`);
  process.exit(1);
}
const events = content
  .trim()
  .split("\n")
  .filter(Boolean)
  .map((line) => JSON.parse(line));
const names = new Set(events.map((event) => event.hook_event_name));
const missing = ["SessionStart", "SessionEnd", "UserPromptSubmit", "Stop"].filter(
  (event) => !names.has(event),
);
if (missing.length > 0) {
  throw new Error(`${provider} did not emit portable hooks: ${missing.join(", ")}`);
}
console.log(`${provider}: ${[...names].join(", ")}`);
