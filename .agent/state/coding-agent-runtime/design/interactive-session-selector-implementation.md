# Interactive Session Selector Implementation

## Conclusion

Bare `ax` uses one grouped session selector on `main`. Commit `139f85b` introduced the grouped interaction; commit `146fda7` replaced its expensive startup path with bounded recent-session discovery. `Current workspace` and `Global` render as non-selectable headings, while a single flattened selection index lets Up/Down move across their boundary and Enter select the highlighted session. `q`, `Q`, Escape, and Ctrl-C cancel. Each discovered summary is classified into exactly one group by whether its workspace matches the current workspace; a Current item is not subsequently considered for Global.

Each empty group renders `No recent sessions`; when both are empty, selection ends with `no recent sessions found in the current workspace or globally`. Rendering writes CRLF and uses a line-counted ANSI redraw after movement so raw terminal mode does not staircase output.

The selector only chooses a summary from the native read-only session service. Full transcript parsing is deferred until the selected session is loaded through `Info`; no selector state or alternate session store was introduced.

## Recent discovery and presentation

`Service.RecentGroups` performs one lightweight discovery pass and returns recent-first Current and Global groups capped at ten entries per provider. Codex, Claude, and Pi JSONL discovery scans only far enough to obtain metadata and the first valid user title; it obtains updated time from records near the file tail, falling back to file modification time. OpenCode discovery reads session info plus the first message that contains a path. The selector displays a loading state before this scan begins.

Recent summaries carry `StartedAt` as well as `UpdatedAt`. Codex, Pi, OpenCode, and Gemini use their native start timestamps. Claude discovery fills `StartedAt` from the first record that contains a timestamp, while retaining the bounded scan.

Recent discovery does not de-duplicate by session ID, source, or visible metadata. Each discovered summary is processed independently, so distinct native sessions remain distinct even when their provider, first valid user-message title, and minute-level displayed update time are identical; their rows can therefore look duplicated. The UI does not expose session IDs. Current/Global separation is an exclusive workspace classification implemented by `matchesWorkspaceSummary` and `continue`, not an ID-based de-duplication pass.

Codex title discovery skips injected context beginning with `# AGENTS.md instructions` or `<environment_context>`. Display shortening in `oneLine` and `singleLine` operates on runes, so multibyte UTF-8 titles are not split.

Session times render in local friendly form: Today, Yesterday, month/day within the current year, and a year-bearing form across years. When `StartedAt` and `UpdatedAt` form a valid non-negative interval, the label appends their difference as a compact session span, for example `Today 17:50 (42m)`, `(1h 07m)`, or `(3d 02h)`. Missing, invalid, or reversed timestamps omit the duration. This span describes elapsed wall-clock time between native session timestamps, not measured active work time.

The time column is limited to 25 characters. Current-workspace entries are single-line provider, time, title rows; they neither create nor reserve a workspace column, and their title is limited to 57 characters. Global entries are single-line provider, time, workspace, title rows. Workspace is the third column, limited to 20 characters, and left-truncated so the path suffix remains visible; missing workspace displays `unknown`. The final Global title is limited to 35 characters. Layout and redraw line accounting avoid wrapping at 96 columns.

## Viewport architecture (resolved vertical layout defect)

Commit `0d4fa5e` replaced the unbounded ANSI redraw with a viewport-based renderer.

**Rendering and layout decoupled**: a `contentRow` struct and `buildContentRows()` function pre-compute all content (headings, empty-state lines, session rows) into a flat list built once per render cycle. Every item -- Current or Global -- is a single row. Selection index maps into this flat list via `selectedRowRange()`, which returns (i, i) for the selected item.

**Terminal height detection**: `terminalHeight()` calls `term.GetSize` on Stdin. Non-terminal environments return 0, interpreted as unlimited height (test-compatible).

**Viewport scrolling**: `chooseSession()` maintains a `viewStart` offset. Each render cycle calls `selectedRowRange()` and adjusts `viewStart` so the selected item stays within the visible window. Viewport height = termH - 2 (header) - 2 (scroll indicators).

**Scroll indicators**: `up N more` / `down N more` appear when selectable sessions are hidden above/below the viewport. Counts reflect hidden sessions only (structural rows excluded).

**Test injection**: `App` has an unexported `termHeight int` field for deterministic viewport testing without a real terminal.

## Why

Removing the separate scope prompt makes workspace scope orthogonal to selection. The mutually exclusive workspace classification places each discovered summary in Current or Global, while flattening only selectable rows preserves section presentation and gives keyboard movement one continuous state space. No later identity- or title-based merge combines distinct native sessions that happen to have identical visible metadata.

The bounded discovery path addresses a verified production-scale failure: the user's machine had 751 Codex JSONL files totaling about 928 MB, while the previous implementation fully parsed all providers twice before first render. The title filtering and rune-safe shortening address the observed injected, indistinguishable titles and malformed UTF-8 output. The session span helps distinguish otherwise similar summaries using native temporal metadata; file size was measured and rejected as an unreliable activity signal.

## Verification

Tests cover recent discovery, native and Claude-derived start timestamps, bare-`ax` empty behavior, cross-boundary movement, each singly empty group, both groups empty, cancellation, workspace grouping, titles, time, duration and workspace presentation, invalid-duration omission, UTF-8 truncation, redraw layout, and viewport behavior. Viewport tests (`TestSessionSelectorViewport`: overflow bottom indicator, scroll follow, content-fits-no-indicator) verify scrolling and indicator correctness.

The current selector tests verify that summaries classified as Current are not also placed in Global; they do not establish ID- or semantic de-duplication within either group.

## Files

- `internal/app/session_selector.go`
- `internal/sessions/recent.go`
- `internal/sessions/service.go`
- `internal/sessions/service_test.go`
- `internal/app/app.go`
- `internal/app/app_test.go`
- `README.md`
- `docs/assets/interactive-resume.gif`
