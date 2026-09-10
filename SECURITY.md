# Security policy

## Supported versions

Security fixes are provided for the latest released version of AgentX.

## Reporting a vulnerability

Do not open a public issue. Use GitHub's private vulnerability reporting for this repository:

<https://github.com/ArcheMind/agentx/security/advisories/new>

Include the affected version, operating system, reproduction steps, impact, and any suggested mitigation. You should receive an acknowledgement within seven days. Please allow time for a fix and coordinated disclosure before publishing details.

## Security model

AgentX launches native coding-agent CLIs with the current user's permissions. It does not accept or store credentials, but native commands may access their own credential and session stores. `AX_LOG=debug` and `--verbose` intentionally record raw external command input and output; treat those logs as sensitive.
