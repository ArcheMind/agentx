import { spawn } from "node:child_process";
import { Type } from "typebox";

const SM_BINARY = process.env.SM_BINARY ?? "sm";
const SM_DELEGATION_DEPTH = process.env.SM_DELEGATION_DEPTH ?? "0";

function debug(...values: any[]) {
  if (process.env.SM_LOG === "debug") console.error("sessionman bridge:", ...values);
}

interface JsonRpcResponse {
  id: number;
  result?: any;
  error?: { code: number; message: string; data?: any };
}

function createMcpSession(binary: string, onClose: () => void) {
  const child = spawn(binary, ["mcp", "serve"], {
    stdio: ["pipe", "pipe", "pipe"],
    env: { ...process.env, SM_DELEGATION_DEPTH },
  });

  debug("spawn", binary, ["mcp", "serve"]);
  child.stderr.on("data", (chunk: Buffer) => debug("mcp stderr", chunk.toString()));
  let closed = false;
  let nextId = 1;
  let buffer = "";
  const pending = new Map<number, { resolve: (v: JsonRpcResponse) => void; reject: (e: Error) => void }>();

  child.stdout.on("data", (chunk: Buffer) => {
    buffer += chunk.toString();
    let newline: number;
    while ((newline = buffer.indexOf("\n")) !== -1) {
      const line = buffer.slice(0, newline).trim();
      buffer = buffer.slice(newline + 1);
      if (!line) continue;
      debug("mcp stdout", line);
      try {
        const msg: JsonRpcResponse = JSON.parse(line);
        if (msg.id != null && pending.has(msg.id)) {
          pending.get(msg.id)!.resolve(msg);
          pending.delete(msg.id);
        }
      } catch {}
    }
  });

  child.on("error", (err: Error) => {
    debug("mcp error", err);
    for (const [, p] of pending) p.reject(err);
    pending.clear();
  });

  child.on("close", () => {
    closed = true;
    const err = new Error("sm mcp serve exited");
    for (const [, p] of pending) p.reject(err);
    pending.clear();
    onClose();
  });

  return {
    request(method: string, params?: any): Promise<JsonRpcResponse> {
      if (closed) return Promise.reject(new Error("sm mcp serve exited"));
      const id = nextId++;
      const msg = JSON.stringify({ jsonrpc: "2.0", id, method, params: params ?? {} });
      debug("mcp stdin", msg);
      return new Promise((resolve, reject) => {
        pending.set(id, { resolve, reject });
        child.stdin.write(msg + "\n");
      });
    },
    notify(method: string, params?: any) {
      child.stdin.write(JSON.stringify({ jsonrpc: "2.0", method, params: params ?? {} }) + "\n");
    },
    close() {
      try { child.kill(); } catch {}
    },
  };
}

function mcpResult(resp: JsonRpcResponse): { content: [{ type: "text"; text: string }]; details: undefined; isError?: boolean } {
  if (resp.error) {
    return { content: [{ type: "text", text: resp.error.message }], details: undefined, isError: true };
  }
  const content = resp.result?.content;
  const text = Array.isArray(content)
    ? content.map((c: any) => c.text ?? "").join("\n")
    : JSON.stringify(resp.result);
  const isError = resp.result?.isError === true;
  return { content: [{ type: "text", text }], details: undefined, ...(isError ? { isError } : {}) };
}

export default function (pi: any) {
  let session: ReturnType<typeof createMcpSession> | null = null;
  let ready: Promise<boolean> | null = null;
  let shuttingDown = false;
  const waiters = new Map<string, ReturnType<typeof spawn>>();
  const cursors = new Map<string, number>();

  async function wake(text: string): Promise<boolean> {
    if (shuttingDown) return false;
    try {
      await pi.sendUserMessage(text, { deliverAs: "steer" });
      return true;
    } catch (err) {
      console.error("SessionMan could not deliver an event to Pi:", err);
      return false;
    }
  }

  function closeWaiters() {
    for (const child of waiters.values()) child.kill();
    waiters.clear();
    cursors.clear();
  }

  function armWait(id: string, after: number) {
    if (shuttingDown || !session || waiters.has(id)) return;
    after = cursors.get(id) ?? after;
    cursors.set(id, after);
    const owner = session;
    const child = spawn(SM_BINARY, ["wait", id, "--after", String(after)], {
      stdio: ["ignore", "pipe", "pipe"],
      env: process.env,
    });
    debug("spawn", SM_BINARY, ["wait", id, "--after", String(after)]);
    waiters.set(id, child);
    let stdout = "";
    let stderr = "";
    child.stdout!.on("data", (chunk: Buffer) => { stdout += chunk.toString(); });
    child.stderr!.on("data", (chunk: Buffer) => { stderr += chunk.toString(); });
    child.on("error", (err: Error) => { stderr += err.message; });
    child.on("close", async (code: number | null) => {
      debug("wait exit", id, code, { stdout, stderr });
      if (waiters.get(id) !== child) return;
      if (shuttingDown) return;
      if (code !== 0) {
        waiters.delete(id);
        wake(`SessionMan event wait for ${id} failed: ${stderr || `exit ${code}`}`);
        return;
      }
      const header = stdout.match(/^SessionMan ExternalSubagent (\S+) event seq=(\d+) kind=(report|message)\n/);
      if (!header || header[1] !== id || Number(header[2]) <= after) {
        waiters.delete(id);
        wake(`SessionMan event wait for ${id} returned an invalid event.`);
        return;
      }
      const delivered = await wake(stdout);
      if (session !== owner || waiters.get(id) !== child) return;
      waiters.delete(id);
      if (!delivered) return;
      cursors.set(id, Number(header[2]));
      if (!/^Report status: (failed|cancelled)\./.test(stdout.slice(header[0].length))) {
        try { armWait(id, Number(header[2])); }
        catch (err) { console.error("SessionMan could not re-arm an event wait:", err); }
      }
    });
  }

  function watchResult(text: string) {
    const notes = [...text.matchAll(/`sm wait (ax-[a-zA-Z0-9-]+) --after (\d+)`/g)];
    const note = notes.at(-1);
    if (note) armWait(note[1], Number(note[2]));
  }

  async function ensureSession(): Promise<ReturnType<typeof createMcpSession>> {
    if (session && ready) {
      await ready;
      return session!;
    }
    session = createMcpSession(SM_BINARY, () => {
      closeWaiters();
      session = null;
      ready = null;
      if (!shuttingDown) void wake("SessionMan MCP connection exited; its delegated agents were disposed. Start a new delegation to reconnect.");
    });
    ready = session.request("initialize", {
      protocolVersion: "2024-11-05",
      capabilities: {},
      clientInfo: { name: "sessionman-bridge", version: "1.0.0" },
    }).then((resp) => {
      if (resp.error) throw new Error(`initialize failed: ${resp.error.message}`);
      session!.notify("notifications/initialized");
      return true;
    });
    await ready;
    return session!;
  }

  pi.on("session_shutdown", () => {
    shuttingDown = true;
    closeWaiters();
    if (session) { session.close(); session = null; ready = null; }
  });

  function bridgeTool(name: string, label: string, description: string, parameters: any, mcpTool: string, mapArgs?: (params: any) => any) {
    pi.registerTool({
      name, label, description, parameters,
      async execute(_toolCallId: string, params: any) {
        try {
          const s = await ensureSession();
          const args = mapArgs ? mapArgs(params) : params;
          const result = mcpResult(await s.request("tools/call", {
            name: mcpTool,
            arguments: args,
          }));
          if (!result.isError && SM_DELEGATION_DEPTH === "0" &&
              ((mcpTool === "StartExternalSubagent" && args.run_in_background !== false) ||
               mcpTool === "SendExternalSubagentMessage")) {
            watchResult(result.content[0].text);
          }
          return result;
        } catch (err: any) {
          return { content: [{ type: "text" as const, text: `${name} failed: ${err.message}` }], details: undefined, isError: true };
        }
      },
    });
  }

  bridgeTool(
    "delegate", "Delegate to SessionMan",
    "Delegate a task to another AI agent via SessionMan. run_in_background (default true) returns immediately; this bridge automatically delivers report and message events into your conversation. Do not run additional waits. false blocks until the first report. Available presets are configured in the user's SessionMan preset collection.",
    Type.Object({
      description: Type.String({ description: "Short task description" }),
      prompt: Type.String({ description: "Full task prompt for the sub-agent" }),
      model: Type.String({ description: "Preset name from the SessionMan preset collection" }),
      name: Type.Optional(Type.String({ description: "Optional name used to address this agent" })),
      run_in_background: Type.Optional(Type.Boolean({ description: "Default true; false blocks until completion" })),
      isolation: Type.Optional(Type.String({ description: "worktree: run in a new git worktree; omit for no isolation" })),
      mode: Type.Optional(Type.String({ description: "Override preset mode. normal: agent's own config decides. yolo: bypass permission checks." })),
    }),
    "StartExternalSubagent",
    (p) => ({ ...p, run_in_background: p.run_in_background ?? true }),
  );

  bridgeTool(
    "delegate_send", "Send to delegated agent",
    "Send a follow-up message to a running or completed SessionMan ExternalSubagent by name or ID. delivery defaults to steer inside the active turn; queued waits until the current run settles. An idle agent starts a new round in the same context.",
    Type.Object({
      to: Type.String({ description: "SessionMan name or ID" }),
      message: Type.String({ description: "Text task" }),
      summary: Type.Optional(Type.String({ description: "Optional display summary" })),
      delivery: Type.Optional(Type.Union([Type.Literal("steer"), Type.Literal("queued")])),
    }),
    "SendExternalSubagentMessage",
  );

  bridgeTool(
    "delegate_stop", "Stop delegated agent",
    "Cancel a running SessionMan ExternalSubagent by name or ID.",
    Type.Object({ task_id: Type.String({ description: "SessionMan name or ID" }) }),
    "StopExternalSubagent",
  );

  if (process.env.SM_HOST_AGENT_DIR) {
    bridgeTool(
      "message_host", "Message your host",
      "Send a short message to the host that dispatched you while continuing your work. Use for questions, early findings and progress.",
      Type.Object({ message: Type.String({ description: "Message for the host agent" }) }),
      "MessageHost",
    );
  }

  bridgeTool(
    "delegate_list", "List delegated agents",
    "List ExternalSubagent sessions in this SessionMan connection, with status and reports.",
    Type.Object({}),
    "ListExternalSubagentSessions",
  );
}
