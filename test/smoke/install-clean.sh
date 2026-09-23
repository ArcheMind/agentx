#!/bin/sh
set -eu

for agent in claude codex gemini pi; do
	ax agent install "$agent"
	done

claude --version
codex --version
gemini --version
pi --version
