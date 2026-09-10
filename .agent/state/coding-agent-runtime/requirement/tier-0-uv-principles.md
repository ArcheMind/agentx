# Tier 0: Product Principles Derived from uv

These are governing product principles, not a claim that the Coding Agent Runtime must copy uv's implementation.

## Core thesis

uv's visible advantage is Rust-driven speed, but its product advantage is redefining Python packaging from a collection-of-tools problem into a developer-experience problem.

Its adoption strategy is:

> Enter through a nearly zero-migration-cost wedge with a 10x–100x experiential advantage, then progressively absorb the surrounding toolchain.

## Principles

1. **Standards maturity creates the consolidation window.** uv did not invent a new packaging standard. It built on the standardized ecosystem data layer (`pyproject.toml` and relevant PEPs) once that layer was mature enough to support a unified client.
2. **Do not own the format; own the workflow.** Prefer existing ecosystem standards and native formats over a proprietary replacement. Compete on the client and workflow experience.
3. **Use a zero-migration wedge.** The initial adoption step must preserve existing projects and semantics while delivering an immediately visible improvement.
4. **Land and expand.** Start with one frequent, painful, high-ROI operation, earn distribution, then extend into adjacent workflow stages.
5. **Unified but modular.** The system may cover a broad workflow, but each capability must remain independently adoptable. Users should be able to start with a small fraction of the platform.
6. **Performance can change the abstraction.** Making an operation cheap enough changes how often it is used and allows it to become a routine runtime primitive rather than an occasional maintenance action.
7. **Decouple adoption cost from ambition.** Platform ambition may be large, but a user's initial commitment must remain small and reversible.

## Priority governance

These principles constrain product shape; they do not determine implementation priority. In particular, `land and expand` must not be interpreted as an instruction to defer modules that the user has prioritized. The user owns priority decisions. What transfers directly from uv is the requirement that the unified system remain modular and support gradual adoption.

## Product test

Any proposed Coding Agent Runtime feature should be evaluated against these questions:

- Does it reuse native standards and formats, or unnecessarily introduce a new one?
- Can a user adopt it without migrating the project or surrendering the native tool?
- Is the entry operation frequent and the benefit immediately observable?
- Can the capability be used independently while still contributing to a unified workflow?
- Does it reduce uncertainty or friction enough to change routine behavior?
- Is the first commitment materially smaller than the long-term platform ambition?
