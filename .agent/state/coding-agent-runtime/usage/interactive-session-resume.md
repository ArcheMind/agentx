# Interactive Cross-Agent Resume

Run bare `ax` inside a workspace to browse recent sessions. Use Up/Down to move through the selectable rows under `Current workspace` and `Global`; headings and empty-state rows are skipped. Press Enter to continue with the highlighted session, or use `q`, Escape, or Ctrl-C to cancel.

AgentX shows a loading status while it performs bounded recent-session discovery. Results are recent-first, with at most ten sessions per provider in each group. Current-workspace rows display provider, local time with an optional session span, and title without reserving space for a workspace column. Global rows display provider, local time with an optional session span, compact suffix-preserving workspace, and title in that order, using `unknown` when workspace is unavailable. The span is `UpdatedAt - StartedAt`; it is omitted when either timestamp is unavailable or the interval is invalid, and represents elapsed session time rather than measured active work.

After selecting a session, choose one of the installed target Agents and then a model when that Agent exposes model choice. AgentX reads the selected native session, creates a bounded normalized transcript, and launches the target Agent in the session workspace without modifying the source store.

`Global` contains summaries whose workspace is classified outside the current workspace. Each discovered summary is assigned to Current or Global, not both. If one group is empty it remains visible as `No recent sessions`. If both groups are empty, `ax` exits with an explicit no-recent-sessions error.

Rows with the same provider, displayed minute, and title can represent different native sessions. There is also a known Codex continuation defect: when a JSONL file contains multiple top-level `session_meta` records, bounded discovery can retain the first ID while full loading overwrites it with a later embedded old ID. Different files may then collapse to one ID and fail as ambiguous even with `--source codex`; the selector does not display enough identity information to distinguish this case visually.
