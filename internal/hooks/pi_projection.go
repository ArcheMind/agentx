package hooks

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

func piExtension(executable, project string, hooks []Hook) string {
	type spec struct {
		Command string  `json:"command"`
		Matcher string  `json:"matcher,omitempty"`
		Timeout float64 `json:"timeout"`
		Project string  `json:"project,omitempty"`
		Payload string  `json:"payload"`
	}
	configured := map[string][]spec{}
	for _, item := range hooks {
		handler := projectedHandler(item, project)
		data, _ := json.Marshal(handler)
		configured[item.Event] = append(configured[item.Event], spec{
			Command: handler.Command, Matcher: handler.Matcher, Timeout: handler.Timeout,
			Project: handler.Project, Payload: base64.RawURLEncoding.EncodeToString(data),
		})
	}
	configuredData, _ := json.Marshal(configured)
	executableData, _ := json.Marshal(executable)
	return fmt.Sprintf(`import { spawn } from "node:child_process";

const executable = %s;
const configured: Record<string, Array<{ command: string; matcher?: string; timeout: number; project?: string; payload: string }>> = %s;

export default function (pi: any) {
  let pendingContext: string[] = [];

  const visible = (ctx: any, message: string, kind = "info") => {
    if (!message) return;
    if (ctx.hasUI) ctx.ui.notify(message, kind);
    else console.error(message);
  };

  const invoke = async (event: string, payload: Record<string, unknown>, ctx: any, spec: { command: string; matcher?: string; timeout: number; project?: string; payload: string }) => {
	const result = await new Promise<{ code: number; stdout: string; stderr: string }>((resolve) => {
	  const args = ["hook", "__dispatch", "--provider", "pi", "--event", event, "--handler", spec.payload];
	  const child = spawn(executable, args, {
        cwd: ctx.cwd,
        stdio: ["pipe", "pipe", "pipe"],
      });
      let stdout = "";
      let stderr = "";
      child.stdout.on("data", (chunk) => stdout += chunk.toString());
      child.stderr.on("data", (chunk) => stderr += chunk.toString());
      child.on("error", (error) => resolve({ code: 1, stdout, stderr: String(error) }));
      child.on("close", (code) => resolve({ code: code ?? 1, stdout, stderr }));
      child.stdin.end(JSON.stringify(payload));
    });
    let output: any = {};
    if (result.stdout.trim()) {
      try { output = JSON.parse(result.stdout); }
      catch { output = { systemMessage: result.stdout.trim() }; }
    }
    if (result.stderr.trim()) console.error(result.stderr.trim());
    visible(ctx, output.systemMessage || "");
    return { ...result, output };
  };

  const base = (nativeEvent: string, ctx: any) => ({
    hook_event_name: nativeEvent,
    session_id: ctx.sessionManager.getSessionId(),
    cwd: ctx.cwd,
    transcript_path: ctx.sessionManager.getSessionFile(),
    model: ctx.model?.id,
    agent: "pi",
  });

  if (configured.SessionStart?.length) {
	pi.on("session_start", async (event: any, ctx: any) => {
	  const results = await Promise.all(configured.SessionStart.map((spec) => invoke("SessionStart", { ...base("session_start", ctx), reason: event.reason }, ctx, spec)));
	  for (const result of results) {
		const context = result.output?.hookSpecificOutput?.additionalContext;
		if (context) pendingContext.push(context);
	  }
	});
  }

  if (configured.SessionEnd?.length) {
	pi.on("session_shutdown", async (event: any, ctx: any) => {
	  await Promise.all(configured.SessionEnd.map((spec) => invoke("SessionEnd", { ...base("session_shutdown", ctx), reason: event.reason }, ctx, spec)));
	});
  }

  if (configured.UserPromptSubmit?.length) {
	pi.on("input", async (event: any, ctx: any) => {
	  const results = await Promise.all(configured.UserPromptSubmit.map((spec) => invoke("UserPromptSubmit", { ...base("input", ctx), text: event.text }, ctx, spec)));
	  const blocked = results.find((result) => ["block", "deny"].includes(String(result.output?.decision || "").toLowerCase()));
	  if (blocked) {
		visible(ctx, blocked.output?.reason || "Hook blocked the turn.", "error");
		return { action: "handled" };
	  }
	  if (results.some((result) => result.code !== 0)) visible(ctx, "Portable hook failed; the turn will continue.", "warning");
	  for (const result of results) {
		const context = result.output?.hookSpecificOutput?.additionalContext;
		if (context) pendingContext.push(context);
	  }
	  return { action: "continue" };
	});
  }

  if (configured.SessionStart?.length || configured.UserPromptSubmit?.length) {
    pi.on("before_agent_start", async (_event: any, _ctx: any) => {
      if (pendingContext.length === 0) return;
      const content = pendingContext.join("\n");
      pendingContext = [];
      return { message: { customType: "agentx-hooks", content, display: true } };
    });
  }

  if (configured.Stop?.length) {
    pi.on("agent_settled", async (_event: any, ctx: any) => {
      const branch = ctx.sessionManager.getBranch();
      const entry = [...branch].reverse().find((item: any) => item.type === "message" && item.message?.role === "assistant");
      const content = Array.isArray(entry?.message?.content)
        ? entry.message.content.filter((item: any) => item.type === "text").map((item: any) => item.text).join("\n")
        : "";
	  await Promise.all(configured.Stop.map((spec) => invoke("Stop", { ...base("agent_settled", ctx), last_assistant_message: content, outcome: "completed" }, ctx, spec)));
	});
  }
}
`, string(executableData), string(configuredData))
}
