# Interactive Session Selector and Demo Redesign

Status: planned for a new session.

## User-reported problems

1. Session selection does not match the intended interaction. Current-workspace and global sessions should appear as two visibly separated lists inside one selector. Up/down navigation must move through every selectable session and cross the boundary between the two groups.
2. The published GIF renders the terminal text as red, producing an incorrect and distracting visual result.
3. The demo exposes only one target Agent (`opencode`), so it fails to communicate AgentX's cross-Agent workflow and the value of choosing a target.

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
