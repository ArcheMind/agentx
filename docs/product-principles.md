# Product principles

AgentX applies the product philosophy demonstrated by uv to coding-agent workflows. The goal is not to copy uv's feature set; it is to use the same adoption logic.

## Make adoption nearly free

The first useful command must work with tools and data the user already has. AgentX discovers native executables and sessions before asking the user to create anything.

## Own the workflow, not the format

AgentX coordinates native tools. It does not require a universal profile, credential vault, project manifest, or session database. Existing formats remain authoritative.

## Deliver an immediate, legible advantage

One command should replace an error-prone cross-tool step: locate every agent, preview an install, see verified models, launch consistently, or find a prior session.

## Unify without making everything symmetrical

Capabilities are modular. A provider can support model selection without exposing a trustworthy model list. Shared grammar reflects shared user intent, while provider-specific limitations remain visible.

## Expand from a narrow successful workflow

New surface area must follow observed friction. AgentX does not add lifecycle commands merely to complete a CRUD matrix, and it does not normalize destructive operations without shared semantics.

## Use performance to improve interaction

A single native Go binary makes discovery and inspection cheap enough to use routinely. Speed matters because it changes whether a coordination step feels like overhead.

## Separate the large vision from the first commitment

AgentX may eventually coordinate more of the coding-agent lifecycle, but a user can adopt one command without migrating the rest of their environment.

## Product test

A proposed feature should satisfy most of these questions:

1. Does it preserve a mature native standard or source?
2. Can a user adopt it without migrating or abandoning a native tool?
3. Is the benefit visible in the first successful use?
4. Can it stand alone and still compose with the common workflow?
5. Does it remove meaningful uncertainty or friction?
6. Is the initial commitment much smaller than the long-term platform vision?

If the evidence is weak, AgentX should expose the limitation instead of filling the gap with a proprietary abstraction.
