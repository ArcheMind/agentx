// Live regression for the `ax session continue` round-trip contract: a caller
// appends each returned delta to its transcript AS-IS and continues. The
// second turn must still see the first turn's content (a secret passphrase),
// which breaks when the native serialization orphans history records (e.g.
// Claude drops everything off the newest record's parentUuid ancestor chain).
//
// Usage: node test/smoke/continue-roundtrip-live.mjs <ax-binary>
//   AX_CONTINUE_AGENTS=claude[,codex]   agents to run (default: claude)
//   AX_CONTINUE_CLAUDE_MODEL            default: haiku
//   AX_CONTINUE_CODEX_MODEL             default: the user's codex default
//   AX_CONTINUE_PI_MODEL                default: deepseek/deepseek-flash
import { execFileSync } from "node:child_process";
import { mkdtempSync, realpathSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { randomUUID } from "node:crypto";

const ax = process.argv[2];
if (!ax) {
  console.error("usage: node continue-roundtrip-live.mjs <ax-binary>");
  process.exit(2);
}
const agents = (process.env.AX_CONTINUE_AGENTS ?? "claude").split(",").map((a) => a.trim()).filter(Boolean);

function modelArgs(agent) {
  if (agent === "claude") return ["--model", process.env.AX_CONTINUE_CLAUDE_MODEL ?? "haiku"];
  if (agent === "codex") {
    const model = process.env.AX_CONTINUE_CODEX_MODEL ?? "";
    return model ? ["--model", model] : [];
  }
  if (agent === "pi") return ["--model", process.env.AX_CONTINUE_PI_MODEL ?? "deepseek/deepseek-flash"];
  throw new Error(`no model policy for agent ${agent}`);
}

function continueTurn(agent, workspace, messages) {
  const transcript = JSON.stringify({
    id: `roundtrip-${randomUUID()}`,
    provider: "smoke",
    workspace,
    messageCount: messages.length,
    messages,
  });
  const stdout = execFileSync(ax, ["session", "continue", agent, ...modelArgs(agent)], {
    input: transcript,
    encoding: "utf8",
    timeout: 300_000,
  });
  const delta = JSON.parse(stdout);
  if (!Array.isArray(delta.messages) || delta.messages.length === 0) {
    throw new Error(`${agent}: empty delta: ${stdout.slice(0, 500)}`);
  }
  return delta;
}

function texts(messages) {
  return messages.flatMap((message) =>
    (message.content ?? []).flatMap((block) => (block.type === "text" && block.text ? [block.text] : [])));
}

let failed = false;
for (const agent of agents) {
  const workspace = realpathSync(mkdtempSync(join(tmpdir(), "ax-continue-roundtrip-")));
  const passphrase = `PINEAPPLE-${Math.floor(Math.random() * 1e10)}`;
  try {
    const first = [{
      id: "u1",
      role: "user",
      content: [{ type: "text", text: `Remember this secret passphrase: ${passphrase}. Reply with just OK.` }],
    }];
    const delta1 = continueTurn(agent, workspace, first);
    // The real contract: the delta is appended AS-IS (namespaced ids,
    // dangling parentIds, native timestamps and all), then continued.
    const second = [
      ...first,
      ...delta1.messages,
      {
        id: "u2",
        role: "user",
        content: [{ type: "text", text: "What was the secret passphrase I told you? Reply with just the passphrase." }],
      },
    ];
    const delta2 = continueTurn(agent, workspace, second);
    const answer = texts(delta2.messages).join("\n");
    if (!answer.includes(passphrase)) {
      throw new Error(`second turn lost the first turn's context; answer: ${answer.slice(0, 300)}`);
    }
    console.log(`PASS continue round-trip ${agent}: passphrase recalled through delta-as-is append`);
  } catch (error) {
    failed = true;
    console.error(`FAIL continue round-trip ${agent}: ${String(error).slice(0, 800)}`);
  }
}
process.exit(failed ? 1 : 0);
