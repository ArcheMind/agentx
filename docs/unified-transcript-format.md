# Unified Transcript Format

The unified transcript format is owned by the shared protocol repository:

https://github.com/ArcheMind/agentx-protocol

- Spec: `docs/unified-transcript-format.md`
- Normative schema: `schema/unified-transcript.schema.json`
- Go implementation (used by agentx): `github.com/ArcheMind/agentx-protocol/transcript`

agentx imports the protocol types and defines no transcript message shapes of
its own. Format changes land in agentx-protocol first, then agentx upgrades
its pinned module version.
