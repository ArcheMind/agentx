# Verified Pi Custom API Provider Discovery

## Conclusion

Pi custom API providers (for example, DeepSeek) cannot be inferred solely from Pi's native `auth.json`. `ax agent models pi` obtains the configured provider IDs through Pi SDK `ModelRuntime.hasConfiguredAuth`, then performs the existing second filtering pass over the strict `pi --list-models` table. This retains custom API models while excluding providers Pi lists without usable configured authentication. Pi auth status uses the same configured-provider source.

## Why

Pi's model listing includes providers regardless of access. Conversely, custom API configuration is not necessarily represented as a persisted `oauth` or `api_key` record in `auth.json`. Pi SDK's runtime is the authoritative provider-aware source for configured authentication, so it preserves both properties without AgentX reading credentials.

## Evidence

Verified by commit `5b8b14b` (`fix: discover Pi custom API providers`): the embedded SDK adapter creates `ModelRuntime`, filters `getProviders()` by `hasConfiguredAuth(provider.id)`, and returns the IDs. `PiModels` intersects those IDs with `pi --list-models`; tests cover including a DeepSeek custom API provider and excluding unconfigured providers.

## Files

`internal/drivers/pi_models.go`, `internal/drivers/pi_sdk_login.go`, `internal/drivers/pi_sdk_providers.mjs`, `internal/drivers/registry.go`, `internal/drivers/pi_models_test.go`
