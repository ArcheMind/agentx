# Verified DeepSeek Harness (DSH) Support

## Status

DeepSeek Harness (DSH) is integrated on `main` in commit `ad2ff41` as a deliberately minimal, verified native Agent integration.

## Supported behavior

- Detection uses the native `dsh` executable.
- Installation planning uses npm package `@deepseek-ai/dsh`.
- Launching invokes the native executable, honors AgentX's working-directory handling, and passes arguments after `--` through unchanged.

## Explicitly unsupported behavior

DSH does not advertise model listing or model selection, authentication `login`/`status`/`logout`, or session operations through AgentX.

## Why

Official `deepseek-ai/deepseek-harness` documentation and locally installed DSH `0.1.5-rc.1` help/package metadata verify the executable, installation package, and native launch surface. They provide no verification evidence for the excluded capabilities. The registry therefore declares only the independently supported drivers instead of fabricating a symmetric lifecycle.

## Files

`README.md`, `docs/lifecycle-and-protocol-audit.md`, `internal/drivers/registry.go`, `internal/drivers/drivers_test.go`, `internal/app/app.go`
