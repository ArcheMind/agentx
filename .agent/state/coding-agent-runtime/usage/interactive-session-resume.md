# Interactive Cross-Agent Resume

Run bare `ax` inside a workspace to browse recent sessions. Use Up/Down to move through the selectable rows under `Current workspace` and `Global`; headings and empty-state rows are skipped. Press Enter to continue with the highlighted session, or use `q`, Escape, or Ctrl-C to cancel.

AgentX shows a loading status while it performs bounded recent-session discovery. Results are recent-first, with at most ten sessions per provider in each group. Times are formatted in local time; Global rows show their compact workspace on a second line or `unknown` when no workspace is available.

After selecting a session, choose one of the installed target Agents and then a model when that Agent exposes model choice. AgentX reads the selected native session, creates a bounded normalized transcript, and launches the target Agent in the session workspace without modifying the source store.

`Global` contains recent sessions outside the current-workspace result set; it does not repeat sessions already shown above. If one group is empty it remains visible as `No recent sessions`. If both groups are empty, `ax` exits with an explicit no-recent-sessions error.
