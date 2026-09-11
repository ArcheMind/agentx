# Verified DeepSeek Harness (DSH) Support

## Status

DeepSeek Harness (DSH) is integrated on `main` with native AgentX support for launch, model discovery, credential status, and Web UI login.

## Supported behavior

- Detection uses the native `dsh` executable.
- Installation planning uses npm package `@deepseek-ai/dsh`.
- Launching invokes the native executable, honors AgentX's working-directory handling, and passes arguments after `--` through unchanged.
- Model discovery uses DSH's native model directory.
- Credential status is exposed without AgentX reading or storing credentials.
- Login opens DSH's native Web UI entry point; it is API-key configuration, not OAuth.

## Native credential behavior

`dsh web` starts the local Web UI. Users configure a provider API key in **Settings → Models**. The Web credential remote writes only to DSH-managed `$DSH_HOME/.credentials.yaml`; the file is mode `0600`, uses an inter-process write lock, and is watched for hot reload. DSH applies credentials in this order: inherited environment variables, managed `credentials.yaml`, current-directory `.env`, then `$DSH_HOME/.env`. Environment layers are read-only and override credentials entered in the Web UI.

The current AgentX registry declares these verified integrations.

## Explicitly unsupported behavior

AgentX does not expose DSH profile, ACP, SDK, headless, or session operations. Model switching remains DSH profile-native configuration. Sessions remain unsupported: compressed, versioned profile storage is configurable, but has no independent stable command surface.

## Why

The installed DSH `v0.1.5-rc.1` Web app, Models UI, `credentials-local` source, and CLI help establish the API-key workflow and credential precedence. Native source and executable surfaces also provide the model directory, credential-state, and Web UI mappings used by AgentX. Profile configuration remains the authoritative model-selection mechanism. The configurable compressed, versioned profile store does not establish an independent stable session command or session contract for AgentX.

## Files

`README.md`, `docs/lifecycle-and-protocol-audit.md`, `internal/drivers/registry.go`, `internal/drivers/drivers_test.go`, `internal/app/app.go`
