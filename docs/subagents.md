# AgentX MCP ExternalSubagents

AgentX exposes `ExternalSubagent`, `SendExternalSubagentMessage`, `StopExternalSubagent`, and `listExternalSubagentSessions` over stdio MCP.
These are explicitly AgentX tools, not official Claude subagents. The selected
native agent executes directly, without a Claude relay inference call. Claude chooses
the combination and any orchestration; AgentX imposes no review/voting workflow.

## Profiles: one collection for `ax` and MCP

A profile is a named, user-confirmed choice of agent, model and effort plus a
description of when to use it. The same collection serves the bare `ax` home
screen and the `ExternalSubagent` choices; there is no separate MCP list.

```text
~/.agentx/profiles/
  deep-review/
    PROFILE.md
  daily/
    PROFILE.md
```

The directory name is the profile name (an ASCII letter or digit, then letters,
digits, `_` or `-`, at most 64 characters). `PROFILE.md` holds a front matter
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
  `pi` can run as MCP `ExternalSubagent`. Other profiles appear in `ax` only;
  the MCP catalog, tool index and `status` leave them out, and a call naming
  one is rejected as `interactive launch only`.
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

### Create and manage from `ax`

Running `ax` with no arguments shows **New session · Profiles** first, with
`+ Create profile…` as its last entry, then recent sessions for the current
workspace and globally. Enter on a profile starts a new native session in the
current directory with exactly that agent, model and effort; Enter on a recent
session continues it as before. `ax <agent>` direct launch is unchanged.

`+ Create profile…` asks for the agent, the model (listed when the agent exposes
a reliable list; otherwise the limitation is shown and the exact ID is typed),
the effort, a name and a description, then shows a summary with
`Save and start`, `Save`, `Back` (change any entry) and `Cancel`. Saving writes
the directory atomically and runs no inference; listing models only reads model
metadata. Duplicate or invalid names are rejected with the other entries kept.
Cancelling leaves nothing behind.

Press `m` on the home screen to manage the highlighted profile (or choose one):
View (configuration and availability status separately), Edit, Rename,
Duplicate (an independent copy, no inheritance), Delete (asks for the named
profile; never touches sessions, native agents, credentials or running work) and
Start new session. Invalid entries are listed with their error and file
location, can be repaired or deleted, and never hide other profiles or recent
sessions. Hand edits to `PROFILE.md` appear on the next screen or MCP request.

If a native agent is missing at start, AgentX shows the reason and offers to edit
the profile or go back; it never substitutes another agent, model or effort.
Native rejection of a model or effort is the native agent's error.

### Register with Claude

The profiles collection (`~/.agentx/profiles/`) is the whole configuration. There is no AgentX switch or
permission file. Registering the MCP server belongs to Claude, through its own
commands (use the absolute path of `ax`; Claude starts it as given):

```sh
claude mcp add --scope user agentx -- "$(command -v ax)" mcp serve
claude mcp remove --scope user agentx
ax profile list
ax profile show codex-review
ax claude
```

- Removing the entry means new Claude sessions do not start the AgentX server.
  Connections already established keep running until their Claude session ends:
  their ExternalSubagents can finish and report, and `StopExternalSubagent`
  stays available there. Removing never cancels work and does not affect
  Claude native subagents.
- `ax claude` reads that registration. When Claude's user configuration
  (`~/.claude.json`, or `$CLAUDE_CONFIG_DIR/.claude.json`) has an `agentx` entry
  running this `ax` executable with `mcp serve`, `ax claude`
  adds the MCP connection and the named channel opt-in (below);
  `ax claude --subagents` requires it. An `agentx` entry running something else
  is reported as a conflict and never replaced.
- `ax profile list` shows the collection with static readiness, limits and
  invalid entries; `ax profile show <name>` shows a single profile's detail.
  Static checks do not verify login or model access; those are established per
  execution. No native-default model or automatic fallback is used.

New work reads the current collection. Existing ExternalSubagents retain their
original combination: an edited, renamed or deleted profile rejects
continuation and queued work instead of rebinding it, and running work is never
stopped or switched by a change.

### Migrating from `subagents.json`

`~/.config/agentx/subagents.json` (its `enabled` switch, the earlier
`default`/`options` keys and the later `grants`) is no longer read. AgentX
ignores the file if it exists and never rewrites it; you can delete it. To
migrate:

- Move each former option into its own profile once (a former option with
  several `efforts` becomes one profile per effort you use; a former
  `enabled: false` option is simply not moved):

  ```sh
  mkdir -p ~/.agentx/profiles/codex-review ~/.agentx/profiles/pi-review
  printf -- '---\nagent: codex\nmodel: gpt-5.6-luna\neffort: low\n---\n\nResearch and code review using native Codex.\n' > ~/.agentx/profiles/codex-review/PROFILE.md
  printf -- '---\nagent: pi\nmodel: openai-codex/gpt-5.6-luna\n---\n\nIndependent research and code review using Pi.\n' > ~/.agentx/profiles/pi-review/PROFILE.md
  rm ~/.config/agentx/subagents.json
  ```

- A former `enabled: true` becomes the `claude mcp add` registration above (if
  it is not registered already); the earlier `enable`/`disable` subcommands no
  longer exist.
- A former grant's `bounded-write` with `writable_roots` becomes a per-call
  request (`write: true` with `writable_roots`, or `write: true` with
  `isolation: "worktree"`); see [Write scope and isolation](#write-scope-and-isolation).
  Its `approval` has no replacement because `never` was the only value.
- A former grant's `auth.path` becomes the native agent's own variable
  (`CODEX_HOME`, `PI_CODING_AGENT_DIR`) in the environment AgentX runs in, or
  nothing when it was the default directory.

## Write scope and isolation

Write access is chosen by each `ExternalSubagent` call, not configured. The
scope is fixed for that ExternalSubagent: `SendExternalSubagentMessage` and
queued work reuse it and cannot widen it.

| Call | Codex sandbox | Pi |
| --- | --- | --- |
| no `write` (default) | `sandbox_mode="read-only"`: no file writes | Read-only tool allowlist |
| `write: true`, `isolation: "worktree"` | `workspace-write` with `writable_roots=[<worktree>]`, the path AgentX created; `writable_roots` in the call is rejected | Rejected |
| `write: true`, `writable_roots: [...]` (no isolation) | `workspace-write`: in write mode the MCP working directory is always writable as well (native Codex behavior); `writable_roots` grants additional directories, symlinks resolved | Rejected |

- Without isolation, `writable_roots` must be absolute existing directories,
  not `/`. AgentX resolves symlinks and hands the result to the Codex OS
  sandbox, which is the only enforcer. AgentX does not compare path strings
  and does not require the working directory to be inside a root. In write
  mode the MCP working directory is always writable as well (native Codex
  behavior, the same as a native subagent writing its own working directory);
  the field description, limits and every report say so.
- Under isolation no caller path string reaches the sandbox, so spellings of
  the original checkout (case, Unicode NFC/NFD, symlink, firmlink) cannot
  reopen it.
- In every writable scope `/tmp`, `$TMPDIR` and network access from commands
  are excluded; Codex keeps `.git` inside the roots read-only (verified); other
  Codex-protected paths are not separately verified. Reads are not
  path-restricted.
- Pi write requests are rejected before any work starts: Pi has no runtime
  sandbox, and an allowlist that adds `bash`/`edit`/`write` could not bound side
  effects. That gap stays open until Pi, or AgentX around it, provides a real
  boundary.
- `writable_roots` without `write: true`, `write: true` without a scope, and
  invalid roots are rejected before any work starts.

Out-of-scope operations are refused and the refusal is returned to the agent, in
one of two layers. Shell commands fail in Codex's OS sandbox (`operation not
permitted`). `apply_patch` outside the roots is rejected earlier by Codex's
application-level patch approval (`rejected by user approval settings`). Codex
runs with `approval_policy="never"`: no human is asked, and Claude's permission
mode or approvals do not apply. There is no approval path (004 A03 stays open).

Codex runs with `--ignore-user-config --ignore-rules --disable hooks`. MCP
servers, hooks and `notify` programs from the user's `config.toml` run outside
Codex's sandbox and would silently widen the write scope, so they are not
loaded. User-config defaults such as model effort, personality and service tier
therefore also do not apply. Codex's built-in web search remains available.

### Authentication

Authentication is discovered, not configured. Before every turn AgentX selects
the native agent's own credential directory, which is `CODEX_HOME` or
`PI_CODING_AGENT_DIR` from its environment, otherwise `~/.codex` or
`~/.pi/agent` (`AX_SUBAGENT_HOME` never changes it). It then runs
`codex login status` or `pi auth check --json --model <model>` against that
directory with credential-like variables removed. Each report carries an
`Auth (discovered by preflight for this turn):` line with the variable,
directory and login method. If the directory is not logged in, not ready for
the model's provider, or the check fails, the task fails with a reason that
ends `no fallback identity`, and no native execution starts.

Inherited environment variables whose names look like credentials (`*KEY*`,
`*TOKEN*`, `*SECRET*`, `*PASSWORD*`, `*CREDENTIAL*`, `*AUTH*`, `*COOKIE*`,
`AWS_PROFILE`) are removed from both the preflight and the native process.
Without this, Pi would silently use an inherited `OPENAI_API_KEY` when the
directory has no login. AgentX never copies credentials between runtimes.
Identity means the directory plus its login method (for Pi also the model's
provider). The account inside that directory is not checked: switching accounts
within the same `CODEX_HOME` or Pi agent directory is not detected.

## Claude discovery and background delivery

MCP initialization includes a profile DATA catalog read from the collection:
names, execution agents, exact models, effort, the user's descriptions, whether
each profile is callable as ExternalSubagent, its write and auth behavior with
the enforcing layer, static prerequisites, and invalid entries with their
location. The same collection supplies the selectable `model` values (callable
profiles only). `ExternalSubagent` tool metadata
carries only a compact index (one line per profile: agent, model,
effort, callability, description) because hosts truncate tool descriptions
(Claude Code keeps 4096 characters); limits are only in
the initialization data. With no profiles the catalog says so and that the user
creates them in `ax`; none are generated. Static checks do not verify login or
model access. Session listing reports created objects, not profiles.
A call result notes when profiles changed since initialization.
Initialization instructions are a connection snapshot. After saving, renaming or
deleting a profile, `ax` says whether running AgentX MCP connections still show
the old list and to reconnect (`/mcp`) or restart Claude, or that AgentX is not
registered with Claude. New calls always use the saved profile; a call
result notes when the collection changed since the connection initialized. A
fresh `tools/list` reads the current collection.

After registering, restart or reconnect the host for tool discovery. `ax claude`
adds the MCP connection and allows exactly the four AgentX tools for that session
with `--allowedTools`; Claude's persistent `settings.json` is not modified.
Background delivery additionally needs Claude's channel opt-in, which AgentX does
not add. That opt-in alone is:

```sh
claude --dangerously-load-development-channels server:agentx
```

The channel path requires interactive Claude Code, first-party provider access,
host feature approval, and organization approval where applicable. AgentX
negotiates MCP `2024-11-05` (legacy); modern protocol revisions do not support
this unsolicited path in Claude Code 2.1.296. Tool discovery alone does not prove
channel delivery. `status` distinguishes the MCP registration, live MCP server
connections, static native prerequisites, and unverified host channel delivery.
Each tool response and background report includes a connection ID. Match it to
`status.connections` to identify the current host, its client name/version and
whether it sent `notifications/initialized`. A running server without that
handshake is not reported as an initialized host.
Claude's debug log should show `Channel notifications registered`; an actual
completion report confirms delivery. Headless (print) Claude has no channel
handler, so there a task is a foreground call: `run_in_background:false` waits
for the terminal result and returns it, the same semantics as a native
subagent in headless Claude. Background delivery is an interactive-session
feature, not a pending headless gap. Other MCP clients may consume the named
notification themselves; background push in other hosts is not claimed.

## Tool contract

```text
ExternalSubagent({"description":"Review authentication", "prompt":"Review token refresh logic",
       "model":"codex-review", "name":"reviewer", "run_in_background":true})
ExternalSubagent({"description":"Fix the flaky test", "prompt":"Fix and run the test",
       "model":"codex-review", "isolation":"worktree", "write":true})
SendExternalSubagentMessage({"to":"reviewer", "message":"Now inspect logout handling", "summary":"Check logout"})
StopExternalSubagent({"task_id":"reviewer"})
listExternalSubagentSessions({})
```

`description` and `prompt` are required strings. `model`
is required and names one callable profile; there is no default profile. The profile
fixes agent, model and effort; there is no `effort` parameter and no per-call
override. Reports name the profile, agent, actual model and effort, and a
`Native evidence:` line states the executed effort that the
native records show for that turn. `isolation` accepts only `worktree` (see
below); any other value is rejected before any work starts. `write` (boolean, default false) and `writable_roots`
(absolute directories) request write access for this ExternalSubagent; see
[Write scope and isolation](#write-scope-and-isolation). Reports carry a
`Write scope:` line and the discovered `Auth` line. Names start with an ASCII
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

ExternalSubagents are session-scoped, like Claude Code's native subagents: they
belong to the MCP connection that created them, and listExternalSubagentSessions
shows only those. A reconnect, or a resumed parent Claude session, starts a new
connection with no ExternalSubagents; an old name or ID then fails explicitly
as unknown. Native transcript files may remain on disk, but AgentX does not
restore objects across connections, by design. Session loss or identity
mismatch is failure, not retry.

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
- **Writes.** With `write: true` the only writable root is the worktree path
  AgentX created; `writable_roots` is rejected together with isolation, so no
  caller-written path string reaches an isolated sandbox. Passing such strings
  is what once reopened the original: an earlier mapping took a root spelled
  `/x/REPO` as unrelated on case-insensitive APFS and made the original
  checkout writable. Codex therefore cannot write the original checkout: an
  absolute write there fails with `operation not permitted`. Codex still
  protects the worktree's own `.git` file inside that root (verified). Reads
  are not path-restricted, so the original stays readable.
- **No commits.** The repository's `.git` lies outside the roots, so the agent
  cannot commit. Its work stays as uncommitted changes in the worktree.
- **Read-only.** Without `write` the sandbox is read-only and has no writable
  root.
- **Concurrent creation.** Worktree creation is serialized within one AgentX
  process, because concurrent `git worktree add` in one repository fails
  (git 2.39 reads another worktree's half-written metadata). Separate AgentX
  processes on the same repository can still hit this; the call then fails
  explicitly with no task started.
- **Pi.** Pi never writes (a write request is rejected), so for Pi a worktree
  changes only the working directory. No OS boundary is added.

The worktree is fixed for the ExternalSubagent's lifetime. Continuation reuses
it and cannot rebind it. If the worktree has been removed, continuation fails
and never falls back to the original directory.

Retention: AgentX never removes a worktree, whether the task completed, failed
or was cancelled, and including after the MCP connection ends. This includes a
task whose auth preflight failed: the worktree was already created, no native
execution ran, and the failure report names the worktree path. Continuation
needs the worktree, and any uncommitted work in it belongs to the user. Every
report names the path, branch, base commit and write scope. After
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

## Nested delegation

A dispatched Codex ExternalSubagent automatically has AgentX available as an
MCP server and can call `ExternalSubagent` itself. This gives dispatched agents
the ability to delegate subtasks to other agents without user configuration.

- **Codex only.** AgentX injects its own MCP server configuration into the
  Codex `-c` flags alongside `--ignore-user-config`, so the only MCP server
  available to the dispatched Codex is AgentX. Pi has no MCP server support.
- **Depth limit.** Delegation depth is tracked via `AX_DELEGATION_DEPTH` and
  capped at 2. A depth-0 server (the top-level connection from Claude Code)
  injects depth 1; a depth-1 server injects depth 2; a depth-2 server does
  not inject further. The initialization instructions report the current depth
  and whether further delegation is available.
- **Foreground only.** Nested hosts (Codex exec) have no channel delivery, so
  `run_in_background` is forced to false regardless of the caller's request.
  The dispatched agent waits for each sub-agent to complete before continuing.
- **Independent write scope.** Each nested ExternalSubagent's write scope is
  enforced by its own Codex OS sandbox at call time, the same as a top-level
  call. A read-only parent does not prevent a writable nested call (same as
  how Claude Code orchestrates top-level calls).
- **Independent auth.** The nested AgentX discovers credentials per execution
  through the same `CODEX_HOME` / `PI_CODING_AGENT_DIR` mechanism. Credential
  environment variables are stripped as usual.
- **`AX_SUBAGENT_HOME`.** If set in the parent process, it is propagated to
  the nested MCP server so the same profile collection is used.

## Native execution and permission limits

| Adapter | Boundary | Native continuation | Effort |
| --- | --- | --- | --- |
| Codex | Call-time write scope as native OS sandbox (`read-only`, or `workspace-write` + roots), `approval_policy="never"`, user config/rules/hooks not loaded, discovered `CODEX_HOME` | Exact thread UUID | Profile effort via `model_reasoning_effort`, or native default; each turn checked against `response.completed` telemetry; native errors remain errors |
| Pi | Read-only only: `--tools read,grep,find,ls --no-extensions --no-skills --no-prompt-templates --no-approve`, discovered `PI_CODING_AGENT_DIR`; with an explicit effort, only AgentX's observer extension is added (`--extension`) | Dedicated exact session path and pinned native ID | Profile effort via `--thinking` (`low`..`max`), or native default; each turn checked against the session thinking level and every provider request |

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
  if the session's thinking level equals the profile effort and every provider
  request of that turn carried exactly that effort. Models whose requests carry
  no reasoning effort field therefore fail explicit effort.

Verified per level on 2026-10-21 (Codex 0.163.0, Pi 0.84.4, see
[acceptance evidence](subagents-acceptance.md)):

| Combination | Runs as | Rejected |
| --- | --- | --- |
| Codex `gpt-5.6-luna` | `none`, `low`, `medium`, `high`, `xhigh`, `max` | `minimal` (native 400), `ultra` (silently run as `max`; detected) |
| Pi `openai-codex/gpt-5.6-luna` | `low`, `medium`, `high`, `xhigh`, `max` | `off`, `minimal` at profile validation (not sent as requested) |
| Pi `deepseek/deepseek-flash` | not claimed | `max` clamped by Pi to `high`; detected |

Other models are checked the same way at run time; no level is claimed for
them in advance.

Pi's allowlist is an application-level tool filter, not an OS sandbox.
Native project context (for example AGENTS.md) still applies. Credentials
and policies are not inherited from Claude. AgentX does not widen permissions for deprecated `mode`. Other
agents (including Gemini) remain available for ordinary AgentX launch/exec but
are not delivered MCP combinations in this integration.

Reports identify profile, execution agent, actual selected model, effort, write scope, discovered auth, ExternalSubagent/task
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
make smoke-profiles
make smoke-mcp-live
make smoke-mcp-channel
make smoke-mcp-host-live
make smoke-mcp-policy-live
make smoke-mcp-effort-live
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
Codex with call-time `write`/`writable_roots` (allowed roots, the implicitly
writable working directory, outside paths, `.git`, `/tmp`, `apply_patch`) and
without write, read-only Pi, a rejected Pi write request, and a second server
whose `CODEX_HOME`/`PI_CODING_AGENT_DIR` are empty, where auth fails before any
native run. Each Codex probe is one command in its own ExternalSubagent. Codex code
mode keeps nested shell and patch calls out of `exec --json`, so the smoke reads
each probe's rollout (`$CODEX_HOME/sessions/**/rollout-*-<thread>.jsonl`). From
it, the smoke asserts the attempt and the runtime's denial, then checks
filesystem state. Override `AX_MCP_CODEX_HOME` and `AX_MCP_PI_AGENT_DIR` to
select the credential directories the live smokes pass to the server as
`CODEX_HOME`/`PI_CODING_AGENT_DIR` (defaults: `~/.codex`, `~/.pi/agent`).

`smoke-mcp-effort-live` runs every Codex level and Pi
`low`..`max` on gpt-5.6-luna, a Pi profile that Pi clamps
(`AX_MCP_PI_CLAMP_MODEL`, default `deepseek/deepseek-flash`, whose credential
must be in the same Pi agent directory), and direct Pi `off`/`minimal` runs with the observer to show why those
stay rejected. Evidence is written to `evidence.json` in the printed directory.

`smoke-mcp-isolation-live` starts from a dirty original checkout and runs real
Codex and Pi with `isolation:"worktree"`. It checks five Codex cases: a write
inside the worktree, an absolute write to the original (OS-denied per rollout),
read-only in a worktree, native failure, and cancellation after partial output.
It also checks Pi's working directory and that an isolation value other than
`worktree` is rejected. A sixth
Codex case writes through a case-variant spelling of the repository: the
sandbox arguments must contain exactly the worktree and no repository path,
and the write must be OS-denied per rollout. A call passing `writable_roots`
together with isolation must be rejected. A seventh case writes the worktree's
own `.git` file and must be denied. Finally it
asserts that the original checkout's status and file contents are byte-identical
and that every worktree is kept.

See [acceptance evidence](subagents-acceptance.md) for scenarios and limitations.

The four names above replace the earlier MCP `Agent`, `SendMessage`, `TaskStop`
and `ListAgents` names. Old names are rejected; there are no compatibility aliases.
The lifecycle is unchanged; `model` now names a profile
from the shared collection and the per-call `effort` key was removed (see
Profiles above). Reconnect Claude MCP after upgrading
to refresh tool metadata; update any explicit MCP tool permission rules to the
new names. Native Claude tools retain their own names and behavior.

Saved host transcripts from before this rename remain historical evidence.
To inspect one, pass `MCP_EVIDENCE_FORMAT=historical` to
`make verify-mcp-host-evidence`; the default parser and current smoke harnesses
use the new names. No new live model run is implied by renaming a harness.
`smoke-mcp-contract` checks the built binary over stdio without creating work;
set `AX_MCP_BINARY` to verify an installed binary through the same Make target.

`make smoke-mcp-profiles` records an isolated real Claude natural-language
observation with randomized profile names. Override `MCP_PROFILE_PROMPT` for
reviewer-owned questions; default judgment records content without a prescribed
answer. The host starts in the temporary directory, not the source repository
(`MCP_PROFILE_CWD` can override it). Only ToolSearch and readonly external-session
listing calls are accepted.
`MCP_PROFILE_JUDGMENT=profiles` enables an optional fixed profile-answer diagnostic,
not a substitute for semantic review. Evidence records host version/model,
answer and tool calls; raw private logs stay in the printed temporary directory.
