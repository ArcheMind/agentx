# Interactive Session Selector and Demo Redesign

Status: done. Commits `139f85b`, `146fda7`, and `0d4fa5e` completed grouped selector, bounded discovery, and viewport scrolling. All completion criteria met.

## Viewport defect (resolved)

Commit `0d4fa5e` added viewport scrolling: `contentRow`/`buildContentRows()` pre-compute a flat row list, `terminalHeight()` detects terminal size, `viewStart` tracks the scroll offset, and `selectedRowRange()` ensures the selected item stays visible. Scroll indicators show hidden session counts above/below.

## Completion outcome

- Bare `ax` now has one selector with non-selectable `Current workspace` and `Global` headings and one cursor that crosses the group boundary.
- Global excludes sessions already shown for the current workspace. Either empty section shows `No recent sessions`; both empty returns an explicit error.
- Enter confirms, while `q`, `Q`, Escape, and Ctrl-C cancel. CRLF rendering keeps ANSI redraw correct in raw terminal mode.
- Native session stores remain read-only, and the existing bounded normalized resume handoff remains in use.
- The re-recorded GIF selects a sanitized Codex source session and offers Claude and OpenCode targets, choosing Claude in the primary path. VHS sets an explicit neutral light-gray foreground.
- Tests cover cross-boundary movement, empty groups, cancellation, and Global de-duplication. The README describes and embeds the redesigned interaction.
- The follow-up in `146fda7` replaced two full session-list parses with one bounded `Service.RecentGroups` discovery pass, shows loading before the scan, and defers full transcript loading until selection.
- Discovery returns recent-first Current and Global groups capped at ten sessions per provider. Titles skip known injected Codex context, truncation is rune-safe, times are local and friendly, and Global rows show compact workspace context.
- The 96-column selector layout avoids wrapping and accounts correctly for two-line rows during redraw. The refreshed README GIF passed multi-frame visual inspection.

## Follow-up diagnosis resolved

On the user's machine, 751 Codex JSONL files totaled about 928 MB. The first grouped implementation parsed the complete provider session set twice before rendering, causing bare `ax` to appear hung. It also selected injected `AGENTS.md` context as Codex titles and truncated multibyte titles by byte. Commit `146fda7` resolves these verified causes through bounded metadata discovery, injected-context filtering, and rune-safe shortening.

## User-reported problems

1. Session selection does not match the intended interaction. Current-workspace and global sessions should appear as two visibly separated lists inside one selector. Up/down navigation must move through every selectable session and cross the boundary between the two groups.
2. The published GIF renders the terminal text as red, producing an incorrect and distracting visual result.
3. The demo exposes only one target Agent (`opencode`), so it fails to communicate AgentX's cross-Agent workflow and the value of choosing a target.
4. The selector renders every candidate at once and does not adapt to terminal height, so a normal terminal cannot show the complete interaction.

## Product intent

Bare `ax` should make recent work directly browsable. Scope is presentation metadata, not a separate interaction state: the user navigates one session candidate set with `Current workspace` and `Global` section boundaries, selects a session, then chooses an Agent and model.

The implementation should first consider subtracting the standalone scope-selection step. Do not add configuration, persistent UI state, or a parallel session model.

## Required interaction

- Render `Current workspace` and `Global` as distinct, non-selectable section headings in one interactive session selector.
- Let one cursor move with up/down across all session rows, including across the section boundary.
- Select the highlighted session with Enter.
- Preserve recent-first ordering within each section.
- Preserve native session stores as read-only sources and reuse the existing bounded resume handoff after selection.
- Decide during design whether the Global section excludes current-workspace sessions or repeats them; document the chosen semantics before implementation.
- Define explicit behavior when either section or both sections are empty.

## Demo requirements

- Re-record the VHS demo after the real selector is implemented; the GIF must demonstrate the actual interaction rather than the old sequential scope prompt.
- Use a neutral, readable terminal palette. Verify the rendered GIF as embedded by GitHub/README, not only its first local frame; no all-red text is acceptable.
- Show multiple installed target Agents so the choice is meaningful.
- Make the cross-Agent value legible: the selected source session and chosen target Agent should not be the same in the primary path.
- Keep all session titles, IDs, timestamps, paths, models, and Agent availability deterministic and sanitized.
- Keep the Tape reproducible without reading native user sessions, credentials, configuration, or installed Agent state, and without launching a real Agent.

## Expected affected areas

- `internal/app/app.go`
- `internal/app/app_test.go`
- `internal/sessions/recent.go`
- `internal/sessions/service.go`
- `internal/sessions/service_test.go`
- Session list grouping or view-model code introduced by the redesign
- `README.md`
- `docs/demo.tape`
- `docs/demo/main.go`
- `docs/demo/bin/*`
- `docs/demo/home/*`
- `docs/assets/interactive-resume.gif`
- Relevant `.agent/state` design records after completion

## Completion criteria

- Bare `ax` presents one keyboard-navigable selector containing distinct Current and Global sections.
- Up/down movement crosses the section boundary and Enter selects the highlighted session.
- Empty-section and cancellation behavior are covered by tests.
- The regenerated GIF has readable neutral colors when viewed from the README.
- The demo visibly offers multiple target Agents and completes a cross-Agent resume path.
- No private native data appears in the Tape, fixtures, generated GIF, or repository history.
- README describes and displays the redesigned interaction.
- `make verify` passes.
- Bare `ax` renders a loading state promptly and does not fully parse all transcripts before showing recent candidates.
- Session titles remain distinctive and valid UTF-8, and the selector stays stable at 96 columns.
- The selector detects available terminal height, renders a bounded viewport, and keeps the selected session visible while navigating within and across groups.
