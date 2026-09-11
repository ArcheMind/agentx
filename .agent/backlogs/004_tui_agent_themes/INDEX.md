# TUI Agent Theme Colors

Status: completed and released in `v0.2.1` on 2026-09-11. Implementation commit: `1e32b4a`.

## User goal

Make the all-white `ax` TUI easier to scan by giving each supported agent a distinct theme color and adding restrained visual hierarchy.

## Completion outcome

- Claude, Codex, DSH, Gemini, OpenCode, and Pi each have a centralized 256-color terminal theme.
- Session rows color only the provider identity; timestamps and workspaces are dimmed, while the selected marker and title receive emphasis.
- Selector titles and section headings are bold. Usage guidance, empty states, and viewport indicators are dimmed.
- Agent selection reuses the same theme mapping. Generic model options receive selection emphasis without being assigned an agent color.
- ANSI styling is disabled for non-terminal output, `NO_COLOR`, and `TERM=dumb`; the existing plain-text layout and visible widths remain unchanged.
- Shared ANSI behavior was moved out of the readiness overview into one terminal-style module instead of creating a second styling system.
- Tests cover all six agent colors, hierarchy styling, generic-option behavior, and ANSI-free fallback output.
- `make verify` passed before release.
- GitHub Release `v0.2.1` completed successfully with Darwin, Linux, and Windows artifacts for amd64/arm64 plus `checksums.txt`.

## Why

The selector previously stored and rendered every row as undifferentiated text even though `App.Color` and ANSI/`NO_COLOR` handling already existed for `ax list`. Reusing and centralizing that mechanism adds semantic emphasis without adding a TUI dependency or persistent presentation state.

## Affected files

- `internal/app/style.go`
- `internal/app/session_selector.go`
- `internal/app/overview.go`
- `internal/app/app_test.go`
- `CHANGELOG.md`

