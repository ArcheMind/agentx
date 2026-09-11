# Interactive Session-Resume Experience

## Requirement

Bare `ax` is the primary recent-work recovery path. It presents current-workspace and other global sessions in one keyboard-navigable selector, then lets the user choose an installed target Agent and, when applicable, a model.

Scope is presentation metadata rather than independent interaction state. `Current workspace` and `Global` are non-selectable headings; one cursor moves across every session row. The Global group excludes sessions already shown for the current workspace. A group with no items remains visible with an explicit empty state, and no sessions in either group produces an explicit error. Enter confirms; `q`, Escape, and Ctrl-C cancel.

The workflow must retain native session stores as read-only sources and use the existing bounded, normalized transcript handoff. It must not add persistent selector state, a parallel session model, or private-store writes.

Recent-session discovery must be bounded before the first selector render and must not fully parse every transcript merely to produce summaries. Show progress while discovery runs, preserve recent-first order, and cap each provider's Current and Global results at ten. Load the full transcript only after selection.

Titles must prefer actual user tasks over injected Agent context, and all truncation must preserve UTF-8 characters. Times use local, relative-friendly labels followed by the session span (`UpdatedAt - StartedAt`) when both timestamps form a valid non-negative duration; missing, invalid, or reversed timestamps omit the span. The time column is limited to 25 characters. Current-workspace rows use provider, time, and title columns only; they do not reserve a workspace column, and the title is limited to 57 characters. Global rows use provider, time, workspace, and title columns in that order; workspace is limited to 20 characters and left-truncates to preserve the path suffix, while title is limited to 35 characters. Global rows explicitly show `unknown` when workspace is unavailable. The selector must remain within a 96-column terminal without corrupting redraw line counts.

The selector must also fit the detected terminal height. It needs a bounded vertical viewport that keeps the selected session visible while moving across group boundaries; headings and workspace context must remain understandable as rows enter and leave the viewport. Rendering must not emit the complete candidate set or rely on full-output line-count rewinds when it exceeds the available height.

## Demo requirement

The README recording must exercise the real selector with deterministic sanitized fixtures, visibly offer multiple target Agents, cross from one Agent's source session to another target Agent, and use a neutral readable terminal foreground. Recording must not read user-native sessions, credentials, configuration, or installed Agent state, and must not launch a real Agent.
