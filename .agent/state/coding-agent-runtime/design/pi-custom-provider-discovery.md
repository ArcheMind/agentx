# Verified Pi Custom API Provider Discovery

## Conclusion

Pi custom API providers (for example, DeepSeek) cannot be inferred solely from Pi's native `auth.json`. `ax agent models pi` asks Pi SDK for each provider's authentication status, then accepts only Pi-stored credentials (`stored`) and `models.json` API-key configuration (`models_json_*`) before performing the existing second filtering pass over the strict `pi --list-models` table. This retains custom API models while excluding unconfigured providers and generic environment-derived providers. Pi auth status uses the same configured-provider source.

## Why

Pi's model listing includes providers regardless of access. Conversely, custom API configuration is not necessarily represented as a persisted `oauth` or `api_key` record in `auth.json`. `ModelRuntime.hasConfiguredAuth` is too broad for AgentX because it accepts generic environment-derived credentials, which caused unrelated provider catalogs to appear. Pi SDK's provider authentication source distinguishes stored and `models.json` credentials without AgentX reading secrets.

## Evidence

Implemented by commit `5b8b14b` (`fix: discover Pi custom API providers`) and corrected after observing generic environment providers in `ax list`: the embedded SDK adapter creates `ModelRuntime`, reads `getProviderAuthStatus(provider.id)`, and returns only `stored` or `models_json_*` IDs. `PiModels` intersects those IDs with `pi --list-models`; tests cover including a DeepSeek custom API provider and excluding unconfigured providers.

The authentication-source correction is commit `508a7c6`. The fix and its documentation are pushed to `origin/main` and published in patch release `v0.2.2` at `5f54619`; release workflow run `34728994409` succeeded.

## Files

`internal/drivers/pi_models.go`, `internal/drivers/pi_sdk_login.go`, `internal/drivers/pi_sdk_providers.mjs`, `internal/drivers/registry.go`, `internal/drivers/pi_models_test.go`
