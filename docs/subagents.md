# AgentX MCP ExternalSubagents

AgentX exposes `ExternalSubagent`, `SendExternalSubagentMessage`, `StopExternalSubagent`, and `listExternalSubagentSessions` over stdio MCP.
These are explicitly AgentX tools, not official Claude subagents. The selected
native agent executes directly, without a Claude relay inference call. Claude chooses
the combination and any orchestration; AgentX imposes no review/voting workflow.

## Presets: one collection for `ax` and MCP

A preset is a named, user-confirmed choice of agent, model and effort plus a
description of when to use it. The same collection serves the bare `ax` home
screen and the `ExternalSubagent` choices; there is no separate MCP list.

```text
~/.agentx/presets/
  deep-review/
    PRESET.md
  daily/
    PRESET.md
```

The directory name is the preset name (an ASCII letter or digit, then letters,
digits, `_` or `-`, at most 64 characters). `PRESET.md` holds a front matter
block and the description:

```markdown
---
agent: codex
model: gpt-5.6-luna
effort: low
---

Read-only research and code review. Not for large refactors.
```

- `agent`: `claude`, `codex`, `gemini`, `opencode` or `pi`. Only `codex` and
  `pi` can run as MCP `ExternalSubagent`; other presets are listed as
  interactive launch only and are never offered as callable.
- `model`: the exact native model ID, passed unchanged to `--model`. Pi needs
  `provider/model`. No fallback model is ever used.
- `effort` (optional): omitted means the agent's native default and is shown as
  "native default". Explicit levels: Codex `none`, `minimal`, `low`, `medium`,
  `high`, `xhigh`, `max`, `ultra` (the creation flow offers only the levels Codex
  lists for the chosen model; a manually entered model shows them as
  unverified); Pi `low`, `medium`, `high`, `xhigh`, `max` (`--thinking`);
  Claude `low`, `medium`, `high`, `xhigh`, `max` (`--effort`; per-model support
  unverified). Gemini and OpenCode accept no explicit effort. Pi `off` and
  `minimal` are rejected: for Codex-backed models Pi sends no effort for `off`
  (the server default, medium for gpt-5.6-luna, applies) and sends `low` for
  `minimal`. As an ExternalSubagent, every turn with an explicit effort is
  checked against native evidence (see
  [Effort verification](#effort-verification)); interactive launch passes the
  level but cannot check it.
- The description is the Markdown body. `name` and `description` keys in the
  front matter and unknown keys are rejected, so there is one authority.

`AX_SUBAGENT_HOME` replaces `~` for both the collection and the AgentX MCP
configuration/state in isolated environments; it does not change native
credentials.

## Roles: `subagent_type`

A role is a named set of task instructions that the native agent actually
follows. Roles are independent of presets: `model` picks the preset (agent,
model, effort, grant), `subagent_type` picks the role, and neither changes the
other. Without any role files only the implicit `general-purpose` role exists,
which adds no instructions.

```text
~/.agentx/roles/
  security-reviewer/
    ROLE.md
```

The directory name is the role name (same rules as preset names; `general-purpose`
is reserved, and the name must match the directory exactly, including case).
`ROLE.md` holds exactly one front matter key and the instructions:

```markdown
---
description: Security review of changed code
---

You are a security reviewer. Focus on injection, authentication and secrets.
Report findings by severity.
```

Any other front matter key (for example `model`, `effort` or `policy`) makes
the role invalid, so a role can never carry agent, model, effort, permission or
credential settings. The instructions are at most 32 KiB. Invalid role files are
listed with their error and location and are not selectable; they never hide
other roles.

How each runtime carries a role, and how AgentX checks it took effect:

| Runtime | Carriage | Per-turn check |
| --- | --- | --- |
| Codex | `-c developer_instructions=<instructions>` (honoured under `--ignore-user-config`) | The rollout (`$CODEX_HOME/sessions/**/rollout-*-<thread>.jsonl`) must contain a `developer` message equal to the instructions |
| Pi | `--append-system-prompt <file>` with a locked copy of the instructions | The AgentX observer extension records, for every provider request, whether the request contains the instructions; all must |

A failed check fails the turn; the text is never silently dropped. With a role,
Pi's own `APPEND_SYSTEM.md` from the grant's agent directory is not loaded for
that run, because the role takes its place in Pi's append slot.

An ExternalSubagent locks the role it was created with, like its preset and
grant. Editing, renaming or deleting the role file rejects continuation and
queued work (`original role <name> changed or was removed`) instead of
switching an existing native session to different instructions. An unknown or
invalid `subagent_type` is rejected before any work starts and no other role
is substituted.

### Create and manage from `ax`

Running `ax` with no arguments shows **New session · Presets** first, with
`+ Create preset…` as its last entry, then recent sessions for the current
workspace and globally. Enter on a preset starts a new native session in the
current directory with exactly that agent, model and effort; Enter on a recent
session continues it as before. `ax <agent>` direct launch is unchanged.

`+ Create preset…` asks for the agent, the model (listed when the agent exposes
a reliable list; otherwise the limitation is shown and the exact ID is typed),
the effort, a name and a description, then shows a summary with
`Save and start`, `Save`, `Back` (change any entry) and `Cancel`. Saving writes
the directory atomically and runs no inference; listing models only reads model
metadata. Duplicate or invalid names are rejected with the other entries kept.
Cancelling leaves nothing behind.

Press `m` on the home screen to manage the highlighted preset (or choose one):
View (configuration and availability status separately), Edit, Rename,
Duplicate (an independent copy, no inheritance), Delete (asks for the named
preset; never touches sessions, native agents, credentials or running work) and
Start new session. Invalid entries are listed with their error and file
location, can be repaired or deleted, and never hide other presets or recent
sessions. Hand edits to `PRESET.md` appear on the next screen or MCP request.

If a native agent is missing at start, AgentX shows the reason and offers to edit
the preset or go back; it never substitutes another agent, model or effort.
Native rejection of a model or effort is the native agent's error.

### Enable MCP

```sh
ax sub-agent-management enable
ax sub-agent-management status
ax claude
ax sub-agent-management disable
```

`~/.config/agentx/subagents.json` holds exactly the feature switch and the
per-preset execution grants (see [Grants](#grants-execution-policy-approval-and-authentication)):

```json
{
  "enabled": true,
  "grants": {
    "codex-review": {
      "execution": {"policy": "read-only", "approval": "never"},
      "auth": {"source": "codex-home", "path": "/Users/me/.codex", "method": "chatgpt"}
    },
    "codex-writer": {
      "execution": {"policy": "bounded-write", "writable_roots": ["/Users/me/work/agentx"], "approval": "never"},
      "auth": {"source": "codex-home", "path": "/Users/me/.codex", "method": "chatgpt"}
    }
  }
}
```

`status` lists the collection with static readiness, each callable preset's
`execution` and `auth`, invalid entries, the declared roles (`roles`,
`invalid_roles`, `roles_directory`) and live MCP connections. Static checks
do not verify remote login or model access; those are established only by
execution. No native-default model or automatic fallback is used.

### Migrating from the earlier `options` configuration

The earlier `default`/`options` keys in `subagents.json` and the `configure`
command no longer exist; a file containing them is reported as invalid. Move each
former option once into the collection, add a grant for each preset under its
name, and drop everything else from the file:

```sh
mkdir -p ~/.agentx/presets/codex-review ~/.agentx/presets/pi-review
printf -- '---\nagent: codex\nmodel: gpt-5.6-luna\neffort: low\n---\n\nRead-only research and code review using native Codex.\n' > ~/.agentx/presets/codex-review/PRESET.md
printf -- '---\nagent: pi\nmodel: openai-codex/gpt-5.6-luna\n---\n\nRead-only independent research and code review using Pi.\n' > ~/.agentx/presets/pi-review/PRESET.md
cat > ~/.config/agentx/subagents.json <<'JSON'
{
  "enabled": true,
  "grants": {
    "codex-review": {
      "execution": {"policy": "read-only", "approval": "never"},
      "auth": {"source": "codex-home", "path": "/Users/me/.codex", "method": "chatgpt"}
    },
    "pi-review": {
      "execution": {"policy": "read-only", "approval": "never"},
      "auth": {"source": "pi-agent-dir", "path": "/Users/me/.pi/agent", "method": "oauth"}
    }
  }
}
JSON
```

Replace the `auth.path` values with your own absolute directories. A former
option with several `efforts` becomes one preset per effort you use, and each of
those presets needs its own grant entry; a preset whose grant is missing is
listed as not callable while the others work. A former `enabled: false` option
is simply not moved.

Enable uses Claude's user-scoped MCP configuration command and preserves other
entries. An identical AgentX entry is reused; a conflicting entry is rejected.
Enable/disable state persists across tasks and sessions. New work reads the
current collection. Existing ExternalSubagents retain their original
combination: an edited, renamed or deleted preset rejects continuation and
queued work instead of rebinding it, and running work is never stopped or
switched by a preset change.

Disabling takes effect for new `ExternalSubagent` and `SendExternalSubagentMessage` submissions immediately.
Previously accepted work, including queued messages, can finish and deliver
results. `StopExternalSubagent` remains available. Disabling does not affect Claude native
subagents or cancel work. Re-enabling keeps the presets.

## Grants: execution policy, approval and authentication

Every MCP-callable preset (agent `codex` or `pi`) needs a grant under
`grants.<preset name>` in `subagents.json` before it can run. Mismatches between
the collection and the grants affect only the entries involved:

- A callable preset without a grant is not callable: it is left out of the
  `model` values, the catalog and `status` show `no grant`, and
  `ExternalSubagent` or `SendExternalSubagentMessage` naming it is rejected with
  that reason. Other presets keep working. Nothing ever runs without a valid
  grant.
- A grant whose `auth.source` belongs to another agent than its preset (for
  example a preset switched from `pi` to `codex` while its grant still uses
  `pi-agent-dir`) makes only that preset not callable, with a `grant mismatch`
  reason naming both agents.
- A grant naming no preset, or naming an interactive-launch-only preset
  (`claude`, `gemini`, `opencode`), has no effect and is reported as a grant
  problem in the catalog and in `status.grant_problems`.
- Interactive-launch-only presets need no grant. A grant for an entry the
  collection reports as invalid waits until that entry is repaired, so one
  broken file never hides the rest.

Structural errors in the security configuration still reject the whole file:
a damaged `subagents.json`, unknown fields, an unsupported `policy`,
`approval`, `auth.source` or `auth.method`, `bounded-write` for Pi, and
`writable_roots` or `auth.path` that are not absolute, clean paths. Every grant
is checked against the agent its `auth.source` implies (so `auth.method` must
fit that source, and `bounded-write` with `pi-agent-dir` is rejected), including
grants that currently have no effect. Then MCP offers no presets, `ExternalSubagent` and
`enable` fail with the reason, and `ax` shows it after every preset change;
`disable` still works.

Grants are kept apart from preset identity (`PRESET.md`: name, agent, model,
effort, description) so presets never carry permissions or credentials; a grant
references its preset only by name. A callable preset created, duplicated or
renamed in `ax` stays not callable until its grant entry is added or renamed,
and `ax` says so after the change; a deleted preset leaves its grant as a
reported problem until removed. Selecting a preset or delegating never grants
more than its grant states.

| Field | Values | Enforced by |
| --- | --- | --- |
| `execution.policy` | `read-only` (Codex, Pi); `bounded-write` (Codex only) | Codex: native OS sandbox (`sandbox_mode` `read-only` / `workspace-write`). Pi: tool allowlist only. |
| `execution.writable_roots` | `bounded-write` only; absolute clean directories, not `/` | Codex `sandbox_workspace_write.writable_roots`; `/tmp`, `$TMPDIR` and command network access excluded |
| `execution.approval` | `never` | Codex `approval_policy="never"`; Pi has no approval path |
| `auth.source` / `auth.path` | Codex `codex-home` (`CODEX_HOME`); Pi `pi-agent-dir` (`PI_CODING_AGENT_DIR`) | Passed as that variable only |
| `auth.method` | Codex `chatgpt` / `api-key`; Pi `oauth` / `api_key` | `codex login status` / `pi auth check --json` before every execution |

`bounded-write` notes. Codex always makes its working directory writable, so
the MCP server's working directory must be inside a writable root. Otherwise
`ExternalSubagent` rejects the call before starting any work. Inside the roots,
Codex keeps `.git` read-only (verified); other Codex-protected paths are not separately verified. Reads are not
path-restricted under either policy. `bounded-write` is rejected for Pi: Pi
has no runtime sandbox, and an allowlist that adds `bash`/`edit`/`write` could
not bound side effects. That gap stays open until Pi, or AgentX around it,
provides a real boundary.

Approval is a user-selected independent policy, not host inheritance. With
`never`, out-of-scope operations are refused and the refusal is returned to the
agent, in one of two layers. Shell commands fail in Codex's OS sandbox
(`operation not permitted`). `apply_patch` outside the roots is rejected earlier
by Codex's application-level patch approval (`rejected by user approval
settings`). No human is asked, and Claude's permission mode or
approvals do not apply. Other approval values (such as on-request routing to a
human or Codex automatic review) are rejected because no approval path is wired.

Codex runs with `--ignore-user-config --ignore-rules --disable hooks`. MCP
servers, hooks and `notify` programs from the user's `config.toml` run outside
Codex's sandbox and would silently widen the grant, so they are not loaded.
User-config defaults such as model effort, personality and service tier
therefore also do not apply. Codex's built-in web search remains available.

Authentication uses only the configured source. Inherited environment
variables whose names look like credentials (`*KEY*`, `*TOKEN*`, `*SECRET*`,
`*PASSWORD*`, `*CREDENTIAL*`, `*AUTH*`, `*COOKIE*`, `AWS_PROFILE`) are removed
from both the preflight and the native process. Without this, Pi would silently
use an inherited `OPENAI_API_KEY` when the selected directory has no login. If
the source is not logged in, not ready for the model's provider, or logged in
with a different method, the task fails with a reason that ends `no fallback
identity`, and no native execution starts. AgentX never copies credentials
between runtimes. Identity means the configured source directory plus its login
method (for Pi also the model's provider). The account inside that source is not
checked: switching accounts within the same `CODEX_HOME` or Pi agent directory
is not detected.

Each callable preset's grant summary and the actual enforcing layer appear in the preset
catalog (MCP initialization and tool metadata), in `status` (`execution`, `auth`)
and in every report's `Grant:` line. An ExternalSubagent pins the grant it was
created with. If the grant changes, continuation and queued work are rejected
instead of running with different permissions or a different identity.

## Claude discovery and background delivery

MCP initialization includes a preset DATA catalog read from the collection:
names, execution agents, exact models, effort, the user's descriptions, whether
each preset is callable as ExternalSubagent, grant summaries with their
enforcing layer, feature enablement, static prerequisites, and invalid entries
with their location. A role DATA block follows: `general-purpose`, each
declared role with its description and full instructions, and invalid role
files. The `subagent_type` enum lists `general-purpose` plus the valid declared
roles. The same catalog supplies `ExternalSubagent` metadata and
its selectable `model` values (callable presets only). With no presets the
catalog says so and that the user creates them in `ax`; none are generated.
Presets without a grant are listed as not callable with the reason, and stray
grants are listed as grant problems. A configuration rejected for a structural
error replaces the catalog with the rejection reason and offers no `model`
values. Static checks do
not verify remote availability. `model` selects a preset independently of the
`subagent_type` role; session listing reports created objects, not presets.
Role file changes are part of the snapshot: a call result notes when presets
or roles changed since initialization.
Initialization instructions are a connection snapshot. After saving, renaming or
deleting a preset, `ax` says whether running AgentX MCP connections still show
the old list and to reconnect (`/mcp`) or restart Claude, which callable
presets still lack a grant, and any stray grant or structural rejection. New calls always use the saved preset; a call
result notes when the collection changed since the connection initialized. A
fresh `tools/list` reads the current collection.

`enable` installs an `agentx` stdio MCP entry pointing to this `ax` executable.
Restart or reconnect the host for tool discovery. `ax claude` automatically adds
the MCP connection and opts in to **only** `server:agentx` using Claude's
`--dangerously-load-development-channels` flag. Claude asks for acknowledgement
of its local-development channel warning. Direct Claude invocation needs:

```sh
claude --dangerously-load-development-channels server:agentx
```

The channel path requires interactive Claude Code, first-party provider access,
host feature approval, and organization approval where applicable. AgentX
negotiates MCP `2024-11-05` (legacy); modern protocol revisions do not support
this unsolicited path in Claude Code 2.1.296. Tool discovery alone does not prove
channel delivery. `status` distinguishes persistent enablement, live MCP server
connections, static native prerequisites, and unverified host channel delivery.
Each tool response and background report includes a connection ID. Match it to
`status.connections` to identify the current host, its client name/version and
whether it sent `notifications/initialized`. A running server without that
handshake is not reported as an initialized host.
Claude's debug log should show `Channel notifications registered`; an actual
completion report confirms delivery. Headless/print Claude does not register
this interactive channel handler: use `run_in_background:false` there. Other MCP
clients may consume the named notification themselves; background push in other
hosts is not claimed.

## Tool contract

```text
ExternalSubagent({"description":"Review authentication", "prompt":"Review token refresh logic",
       "model":"codex-review", "name":"reviewer", "run_in_background":true})
SendExternalSubagentMessage({"to":"reviewer", "message":"Now inspect logout handling", "summary":"Check logout"})
StopExternalSubagent({"task_id":"reviewer"})
listExternalSubagentSessions({})
```

`description` and `prompt` are required strings. `subagent_type` is optional:
`general-purpose` (the default) or a declared role (see [Roles](#roles-subagent_type));
it never selects the execution agent, model, effort or grant. `model`
is required and names one callable preset; there is no default preset. The preset
fixes agent, model and effort; there is no `effort` parameter and no per-call
override. Reports name the preset, agent, actual model, effort and role, and a
`Native evidence:` line states the executed effort and role carriage that the
native records show for that turn. `isolation` accepts the native spellings
`worktree` (supported, see below) and `remote` (rejected, see below); other
values are rejected. Names start with an ASCII
letter or digit and contain 1–64 ASCII letters, digits, `_`, or `-`.
Deprecated `team_name` and native `mode` values (`acceptEdits`, `auto`,
`bypassPermissions`, `default`, `dontAsk`, `plan`) are accepted and ignored;
they do not grant permissions. Unknown properties, nulls, wrong types, and
unsupported values are rejected.

Omitted/true `run_in_background` returns a task identity immediately and pushes
its terminal result. False waits for a terminal result; failure/cancellation is
a tool error, not successful submission. `SendExternalSubagentMessage` accepts text and returns
queued acknowledgement, never a claim of completion. Busy ExternalSubagents process
messages in order using the same native context. Every accepted message has its
own task ID and terminal notification, including cancellation or prior-turn
failure. Completed ExternalSubagents resume via their exact native session.

Every ExternalSubagent call creates an independent object. Reusing a name makes that name
refer to the newest object; previous objects remain addressable by ID. Cancel
targets only that object's process group and queued work, waits for termination,
and distinguishes already-finished and unknown targets. Failed/cancelled agents
cannot resume an uncertain context; the error is explicit and no replacement
session or repeat task is created.

Objects and listExternalSubagentSessions are scoped to the current MCP connection. Reconnecting
loses actionable objects; native transcript files may remain, but AgentX does
not claim cross-connection recovery. This scope also applies when the parent
Claude session resumes. Session loss or identity mismatch is failure, not retry.

## Isolation

`isolation:"worktree"` runs the ExternalSubagent in a new git worktree:

- **What is created.** The repository is the one containing the MCP server's
  working directory, and the worktree starts at its `HEAD` commit. It lives at
  `~/.local/state/agentx/worktrees/<id>` on branch `agentx/<id>`, where `<id>`
  is the ExternalSubagent ID. The native agent's working directory is the
  matching path inside the worktree.
- **The original checkout.** Its files are never modified. Its uncommitted and
  untracked changes are not copied into the worktree. The original repository
  does gain branch `agentx/<id>` and git's worktree metadata under
  `.git/worktrees/`.
- **Rejections.** The call is rejected before any work starts in three cases:
  the working directory is not inside a git work tree, there is no commit at
  `HEAD`, or the working directory is untracked at `HEAD` and therefore
  missing from the worktree. A rejected call leaves no
  worktree or branch behind.
- **Repository hooks.** `git worktree add` runs the repository's own
  `post-checkout` hook, as git always does. That hook is the repository's code,
  not the agent's. It runs outside any agent sandbox, with AgentX's full
  inherited environment, credential-like variables included. Choose worktree
  isolation only for repositories whose hooks you trust.
- **Why the worktree is outside the repository.** A nested worktree would show
  up as an untracked entry in the original checkout, and `git add -A` there
  would record it as an embedded repository.
- **Writable roots under `bounded-write`.** Isolation overrides the grant's
  `writable_roots`: the effective writable root is the worktree itself, and
  nothing else. No granted path reaches an isolated run. Granted paths are not
  mapped into the worktree, not narrowed to it, and not passed through as
  unrelated roots. An isolated task is meant to produce output only in the
  copy, and passing user-written path strings to the sandbox reopens the
  original. An earlier mapping reopened it on case-insensitive APFS: a root
  spelled `/x/REPO` was taken as unrelated and made the original checkout
  writable. Codex therefore cannot write the original checkout or any granted
  path: an absolute write there fails with `operation not permitted`. Codex
  still protects the worktree's own `.git` file inside that root (verified).
  Reads are not path-restricted, so the original stays readable. The grant's
  roots still apply to non-isolated runs, and must still exist.
- **No commits.** The repository's `.git` lies outside the roots, so the agent
  cannot commit. Its work stays as uncommitted changes in the worktree.
- **Read-only.** `read-only` stays read-only: the sandbox is read-only and
  has no writable root.
- **Concurrent creation.** Worktree creation is serialized within one AgentX
  process, because concurrent `git worktree add` in one repository fails
  (git 2.39 reads another worktree's half-written metadata). Separate AgentX
  processes on the same repository can still hit this; the call then fails
  explicitly with no task started.
- **Pi.** Pi is read-only only, so for Pi a worktree changes only the working
  directory. No OS boundary is added.

The worktree is fixed for the ExternalSubagent's lifetime. Continuation reuses
it and cannot rebind it. If the worktree has been removed, continuation fails
and never falls back to the original directory.

Retention: AgentX never removes a worktree, whether the task completed, failed
or was cancelled, and including after the MCP connection ends. Continuation
needs the worktree, and any uncommitted work in it belongs to the user. Every
report names the path, branch, base commit and effective writable roots. After
each task, the report also records the state: whether `HEAD` moved, plus
`git status --porcelain`. It ends with the cleanup command:
`git -C <repo> worktree remove <path>` (which refuses while changes remain; add
`--force` to discard them), then `git -C <repo> branch -D agentx/<id>`. To take
the agent's changes, inspect or copy them from the worktree, or commit them
there yourself.

Bulk cleanup. Retained worktrees accumulate until you remove them. To list
them:

```sh
git -C <repo> worktree list                   # paths and branches, incl. AgentX ones
git -C <repo> branch --list 'agentx/*'        # AgentX branches
ls ~/.local/state/agentx/worktrees            # worktrees from every repository
```

Inspect each one before removing it. Then run `git -C <repo> worktree remove
<path>` and `git -C <repo> branch -D agentx/<id>` for each. If a worktree
directory was deleted by hand, run `git -C <repo> worktree prune` to drop its
stale metadata. The branch stays until you delete it.

`isolation:"remote"` is rejected, and nothing runs locally instead. None of the
installed runtimes offers a remote environment whose identity, result retrieval
and cancellation AgentX can verify:
- Codex 0.163 `codex cloud` can submit, poll, diff and apply tasks, but it has
  no cancel command. Its tasks run in a Codex Cloud environment bound to a
  pushed GitHub branch, not in this working directory.
- `codex exec-server --remote` needs a remote host that the user provisions.
- Pi has no remote execution.

This gap (004 G06/A07) stays open.

## Native execution and permission limits

| Adapter | Boundary | Native continuation | Effort |
| --- | --- | --- | --- |
| Codex | Grant policy as native OS sandbox (`read-only` or `workspace-write` + roots), `approval_policy="never"`, user config/rules/hooks not loaded, `CODEX_HOME` from grant | Exact thread UUID | Preset effort via `model_reasoning_effort`, or native default; each turn checked against `response.completed` telemetry; native errors remain errors |
| Pi | Read-only only: `--tools read,grep,find,ls --no-extensions --no-skills --no-prompt-templates --no-approve`, `PI_CODING_AGENT_DIR` from grant; with a role or explicit effort, only AgentX's observer extension is added (`--extension`) | Dedicated exact session path and pinned native ID | Preset effort via `--thinking` (`low`..`max`), or native default; each turn checked against the session thinking level and every provider request |

### Effort verification

Native CLIs can run a different effort from the one requested without failing,
and their own records may still show the requested one. AgentX therefore
checks what was actually sent or run, per turn, and fails the turn on any
difference. The failed report still carries the agent's text, labelled
untrusted, and the ExternalSubagent cannot be continued; nothing is retried.

- Codex: the configured level is echoed in the rollout `turn_context` even
  when Codex sends another one (`gpt-5.6-luna` with `ultra` sends `max`). AgentX
  starts Codex with `RUST_LOG=codex_otel.trace_safe=info` and reads every
  `codex.sse_event` `response.completed` event, whose `model_reasoning_effort`
  is the level the model response ran with. These trace-safe events carry no
  account or e-mail; they are removed from failure text returned to Claude and
  appear only in the debug log. A level the server does not support fails
  natively (`minimal` on gpt-5.6-luna: HTTP 400, returned as the turn's reason).
  With no explicit effort the executed level is only reported.
- Pi: Pi clamps a level the model cannot use (recorded in the session as
  `thinking_level_change`) and its providers may then remap or drop it
  without any session trace (Codex-backed models: `minimal` is sent as `low`,
  `off` sends no effort). AgentX loads its own observer extension, which only
  appends an `agentx-provider-request` entry with the request's reasoning
  effort to the Pi session before each provider request. A turn passes only
  if the session's thinking level equals the preset effort and every provider
  request of that turn carried exactly that effort. Models whose requests carry
  no reasoning effort field therefore fail explicit effort.

Verified per level on 2026-10-21 (Codex 0.163.0, Pi 0.84.4, see
[acceptance evidence](subagents-acceptance.md#roles-and-effort-backlog-004-g04-g09)):

| Combination | Runs as | Rejected |
| --- | --- | --- |
| Codex `gpt-5.6-luna` | `none`, `low`, `medium`, `high`, `xhigh`, `max` | `minimal` (native 400), `ultra` (silently run as `max`; detected) |
| Pi `openai-codex/gpt-5.6-luna` | `low`, `medium`, `high`, `xhigh`, `max` | `off`, `minimal` at preset validation (not sent as requested) |
| Pi `deepseek/deepseek-flash` | not claimed | `max` clamped by Pi to `high`; detected |

Other models are checked the same way at run time; no level is claimed for
them in advance.

Pi's allowlist is an application-level tool filter, not an OS sandbox.
Native project context (for example AGENTS.md) still applies. Credentials
and policies are not inherited from Claude. AgentX does not widen permissions for deprecated `mode`. Other
agents (including Gemini) remain available for ordinary AgentX launch/exec but
are not delivered MCP combinations in this integration.

Reports identify preset, execution agent, actual selected model, effort, grant, ExternalSubagent/task
IDs, status, failure reason, and continuation/cancellation instructions. Reports
and channels are labelled untrusted ExternalSubagent content and never user
authorization. Available native usage is shown as backend-reported external
usage with native field names; unknown usage stays unknown. Codex resume usage
may include context/cumulative fields; AgentX does not reinterpret it as Claude
usage or fabricate tool counts. `AX_LOG=debug` logs raw command plans, stdin,
stdout, stderr and errors to stderr; it may contain task contents. Values of
credential-like plan environment variables are logged as `<redacted>`, and
`sk-…` keys, JWTs, bearer tokens and JSON token/key fields in logged I/O are
masked. Removed variables are logged by name only (`unset_env`).

## Verification

```sh
make verify
make test-mcp-race
make smoke-mcp-management
make smoke-mcp-contract
make smoke-presets
make smoke-mcp-live
make smoke-mcp-channel
make smoke-mcp-host-live
make smoke-mcp-policy-live
make smoke-mcp-roles-effort-live
make smoke-mcp-isolation-live
```

The live backend suite uses existing native logins, isolated AgentX configuration,
and Codex `gpt-5.6-luna` / Pi `openai-codex/gpt-5.6-luna`. Override
`AX_MCP_CODEX_MODEL` / `AX_MCP_PI_MODEL` with exact accessible models. It verifies
native inference, independent memory continuation and actual cancellation. The
channel suite starts real interactive Claude in a PTY with a deterministic
delayed backend fixture; it requires both channel registration and a subsequent
assistant transcript containing a token absent from its prompt. This separates
host-consumption evidence from actual-backend evidence. Neither fixture tests
nor a server notification alone prove native execution or host delivery.

`smoke-mcp-policy-live` runs real Codex and Pi through AgentX MCP. It covers
bounded-write and read-only Codex, read-only Pi, and missing or mismatched auth
sources. Each Codex probe is one command in its own ExternalSubagent. Codex code
mode keeps nested shell and patch calls out of `exec --json`, so the smoke reads
each probe's rollout (`$CODEX_HOME/sessions/**/rollout-*-<thread>.jsonl`). From
it, the smoke asserts the attempt and the runtime's denial, then checks
filesystem state. Override
`AX_MCP_CODEX_HOME`, `AX_MCP_PI_AGENT_DIR`, `AX_MCP_CODEX_AUTH_METHOD` and
`AX_MCP_PI_AUTH_METHOD` to select the auth sources the live smokes use (defaults:
`~/.codex` chatgpt, `~/.pi/agent` oauth).

`smoke-mcp-roles-effort-live` runs two declared roles and `general-purpose` on
the same Codex and Pi presets and asserts different role behavior from the
reports plus the rollout developer message or Pi observer records, identical
native argv apart from role carriage, role kept across a resume, and
continuation rejected after a role edit. It then runs every Codex level and Pi
`low`..`max` on gpt-5.6-luna, a Pi preset that Pi clamps
(`AX_MCP_PI_CLAMP_MODEL`, default `deepseek/deepseek-flash` with `api_key`
auth), and direct Pi `off`/`minimal` runs with the observer to show why those
stay rejected. Evidence is written to `evidence.json` in the printed directory.

`smoke-mcp-isolation-live` starts from a dirty original checkout and runs real
Codex and Pi with `isolation:"worktree"`. It checks five Codex cases: a write
inside the worktree, an absolute write to the original (OS-denied per rollout),
read-only in a worktree, native failure, and cancellation after partial output.
It also checks Pi's working directory and the rejection of `remote`. A sixth
Codex case grants only a case-variant spelling of the repository plus an
unrelated directory. The sandbox must receive only the worktree, and an
absolute write to the original through that spelling must be OS-denied per
rollout. A seventh case writes the worktree's own `.git` file and must be
denied. Finally it
asserts that the original checkout's status and file contents are byte-identical
and that every worktree is kept.

See [acceptance evidence](subagents-acceptance.md) for scenarios and limitations.

The four names above replace the earlier MCP `Agent`, `SendMessage`, `TaskStop`
and `ListAgents` names. Old names are rejected; there are no compatibility aliases.
The lifecycle is unchanged; `model` now names a preset
from the shared collection and the per-call `effort` key was removed (see
Presets above). Reconnect Claude MCP after upgrading
to refresh tool metadata; update any explicit MCP tool permission rules to the
new names. Native Claude tools retain their own names and behavior.

Saved host transcripts from before this rename remain historical evidence.
To inspect one, pass `MCP_EVIDENCE_FORMAT=historical` to
`make verify-mcp-host-evidence`; the default parser and current smoke harnesses
use the new names. No new live model run is implied by renaming a harness.
`smoke-mcp-contract` checks the built binary over stdio without creating work;
set `AX_MCP_BINARY` to verify an installed binary through the same Make target.

`make smoke-mcp-presets` records an isolated real Claude natural-language
observation with randomized preset names. Override `MCP_PRESET_PROMPT` for
reviewer-owned questions; default judgment records content without a prescribed
answer. The host starts in the temporary directory, not the source repository
(`MCP_PRESET_CWD` can override it). Only ToolSearch and readonly external-session
listing calls are accepted.
`MCP_PRESET_JUDGMENT=presets` enables an optional fixed preset-answer diagnostic,
not a substitute for semantic review. Evidence records host version/model,
answer and tool calls; raw private logs stay in the printed temporary directory.
