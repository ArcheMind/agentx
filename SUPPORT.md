# Support

Use [GitHub Issues](https://github.com/ArcheMind/agentx/issues) for reproducible bugs and focused feature requests.

Before opening an issue:

1. Run `sm version` and record the result.
2. Record the native agent and its version.
3. Reproduce with the smallest possible command.
4. If useful, rerun with `SM_LOG=debug`; remove credentials, prompts, session content, and other sensitive native output before sharing it.

SessionMan can only report capabilities exposed by a verified native source. A missing model list or authentication status may be an intentional limitation rather than a defect; check the [capability audit](docs/lifecycle-and-protocol-audit.md).

Report security issues privately as described in [SECURITY.md](SECURITY.md).
