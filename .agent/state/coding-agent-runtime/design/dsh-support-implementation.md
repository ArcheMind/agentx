# Verified DeepSeek Harness (DSH) Support

## Status

DeepSeek Harness (DSH) is integrated on `main` with native AgentX support for launch, model discovery, credential status, and Web UI login.

## Supported behavior

- Detection uses the native `dsh` executable.
- Installation planning uses npm package `@deepseek-ai/dsh`.
- Launching invokes the native executable, honors AgentX's working-directory handling, and passes arguments after `--` through unchanged.
- Model discovery uses DSH's native model directory.
- Credential status is exposed without AgentX reading or storing credentials.
- Login opens DSH's native Web UI entry point.

The current AgentX registry declares these verified integrations.

## Explicitly unsupported behavior

AgentX does not expose DSH profile, ACP, SDK, headless, or session operations. Model switching remains DSH profile-native configuration. Sessions remain unsupported: compressed, versioned profile storage is configurable, but has no independent stable command surface.

## Why

DSH's native source and executable surfaces provide the model directory, credential-state, and Web UI login mappings used by AgentX. Profile configuration remains the authoritative model-selection mechanism. The configurable compressed, versioned profile store does not establish an independent stable session command or session contract for AgentX.

## Files

`README.md`, `docs/lifecycle-and-protocol-audit.md`, `internal/drivers/registry.go`, `internal/drivers/drivers_test.go`, `internal/app/app.go`
