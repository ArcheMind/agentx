import { spawn } from "node:child_process";
import { Type } from "typebox";

const SM_BINARY = process.env.SM_BINARY ?? "sm";
const SM_DELEGATION_DEPTH = process.env.SM_DELEGATION_DEPTH ?? "1";

interface JsonRpcResponse {
  id: number;
  result?: any;
  error?: { code: number; message: string; data?: any };
}

function createMcpSession(binary: string) {
  const child = spawn(binary, ["mcp", "serve"], {
    stdio: ["pipe", "pipe", "pipe"],
    env: { ...process.env, SM_DELEGATION_DEPTH },
  });

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
    for (const [, p] of pending) p.reject(err);
    pending.clear();
  });

  child.on("close", () => {
    const err = new Error("sm mcp serve exited");
    for (const [, p] of pending) p.reject(err);
    pending.clear();
  });

  return {
    request(method: string, params?: any): Promise<JsonRpcResponse> {
      const id = nextId++;
      const msg = JSON.stringify({ jsonrpc: "2.0", id, method, params: params ?? {} });
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
  const isError = Array.isArray(content) && content.some((c: any) => c.isError);
  return { content: [{ type: "text", text }], details: undefined, ...(isError ? { isError } : {}) };
}

export default function (pi: any) {
  let session: ReturnType<typeof createMcpSession> | null = null;
  let ready: Promise<boolean> | null = null;

  async function ensureSession(): Promise<ReturnType<typeof createMcpSession>> {
    if (session && ready) {
      await ready;
      return session!;
    }
    session = createMcpSession(SM_BINARY);
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
    if (session) { session.close(); session = null; ready = null; }
  });

  function bridgeTool(name: string, label: string, description: string, parameters: any, mcpTool: string, mapArgs?: (params: any) => any) {
    pi.registerTool({
      name, label, description, parameters,
      async execute(_toolCallId: string, params: any) {
        try {
          const s = await ensureSession();
          return mcpResult(await s.request("tools/call", {
            name: mcpTool,
            arguments: mapArgs ? mapArgs(params) : params,
          }));
        } catch (err: any) {
          return { content: [{ type: "text" as const, text: `${name} failed: ${err.message}` }], details: undefined, isError: true };
        }
      },
    });
  }

  bridgeTool(
    "delegate", "Delegate to SessionMan",
    "Delegate a task to another AI agent via SessionMan. run_in_background (default true) returns immediately with a result file path and wait command; false blocks until completion. Available presets are configured in the user's SessionMan preset collection.",
    Type.Object({
      description: Type.String({ description: "Short task description" }),
      prompt: Type.String({ description: "Full task prompt for the sub-agent" }),
      model: Type.String({ description: "Preset name from the SessionMan preset collection" }),
      run_in_background: Type.Optional(Type.Boolean({ description: "Default true; false blocks until completion" })),
      isolation: Type.Optional(Type.String({ description: "worktree: run in a new git worktree; omit for no isolation" })),
      mode: Type.Optional(Type.String({ description: "Override preset mode. normal: agent's own config decides. yolo: bypass permission checks." })),
    }),
    "StartExternalSubagent",
    (p) => ({ ...p, run_in_background: p.run_in_background ?? true }),
  );

  bridgeTool(
    "delegate_send", "Send to delegated agent",
    "Send a follow-up message to a running or completed SessionMan ExternalSubagent by name or ID. The sub-agent resumes in the same context.",
    Type.Object({
      to: Type.String({ description: "SessionMan name or ID" }),
      message: Type.String({ description: "Text task" }),
      summary: Type.Optional(Type.String({ description: "Optional display summary" })),
    }),
    "SendExternalSubagentMessage",
  );

  bridgeTool(
    "delegate_stop", "Stop delegated agent",
    "Cancel a running SessionMan ExternalSubagent by name or ID.",
    Type.Object({ task_id: Type.String({ description: "SessionMan name or ID" }) }),
    "StopExternalSubagent",
  );

  bridgeTool(
    "delegate_list", "List delegated agents",
    "List ExternalSubagent sessions in this SessionMan connection, with status and reports.",
    Type.Object({}),
    "ListExternalSubagentSessions",
  );
}
