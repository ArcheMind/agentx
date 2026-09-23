#!/bin/sh
set -eu

project=/workspace
skill=agentx-unified-smoke
mkdir -p "$project"
cd "$project"

ax skill install /opt/agentx-unified-smoke --scope project

canonical=$project/.agents/skills/$skill
projection=$project/.claude/skills/$skill
[ -f "$canonical/SKILL.md" ]
[ -L "$projection" ]
[ "$(readlink "$projection")" = "../../.agents/skills/$skill" ]

ax --json skill show "$skill" | node -e '
let data = "";
process.stdin.on("data", chunk => data += chunk);
process.stdin.on("end", () => {
  const skill = JSON.parse(data);
  const agents = [...skill.agents].sort().join(",");
  if (skill.name !== "agentx-unified-smoke" || agents !== "claude,codex,gemini,pi") process.exit(1);
});
'
ax --json skill list | node -e '
let data = "";
process.stdin.on("data", chunk => data += chunk);
process.stdin.on("end", () => {
  const result = JSON.parse(data);
  if (!result.skills.some(skill => skill.name === "agentx-unified-smoke")) process.exit(1);
});
'
