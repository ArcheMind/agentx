# Unified Transcript Format

The unified transcript format is owned by the shared protocol repository:

https://github.com/ArcheMind/agentx-protocol

- Spec: `docs/unified-transcript-format.md`
- Normative schema: `schema/unified-transcript.schema.json`
- Go implementation (used by agentx): `github.com/ArcheMind/agentx-protocol/transcript`

agentx imports the protocol types and defines no transcript message shapes of
its own. Format changes land in agentx-protocol first, then agentx upgrades
its pinned module version.

## Antigravity CLI native storage

Antigravity CLI (`agy`) 1.5.8 stores conversations as SQLite databases under
`~/.gemini/antigravity-cli/conversations/<id>.db`. SessionMan reads the native
Step protobuf payloads directly, including committed WAL records. The standalone
native fixture and descriptor field inventory live in
`internal/sessions/testdata/agy/`.

Export uses the same unified transcript schema as every other agent. Images
are embedded as base64; local image and complete tool-result resource files
are resolved during reading, so the exported transcript does not depend on
the original resource paths. Missing resources and unsupported native payloads
produce explicit errors.

Import creates a new conversation UUID, a new native SQLite database and an
entry in `conversation_summaries.db`. It does not clone a source database or
retain a raw database backup in the unified transcript. Source files are only
read. Native conversion output for `agy` is SQLite bytes, not JSON text.

Thinking text and its signature are distinct. Same-provider thinking retains
its native signature; cross-provider conversion clears the foreign signature
and projects readable thought text into `<think>` text. Native signature
acceptance and provider replay are separate facts: an unsigned native thought
is not guaranteed to be replayed by a model. Tool call identifiers remain
linked to their results, including complete results recovered from local
native resource files.

See [Antigravity acceptance](agy-acceptance.md) for the conversion matrix and
live evidence boundaries.
