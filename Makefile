.PHONY: fmt check test test-subagents smoke-subagents smoke-subagents-live audit build install smoke smoke-live smoke-clean smoke-hooks-live smoke-skills-clean smoke-skills-live verify clean
.PHONY: smoke-mcp-management smoke-mcp-live smoke-mcp-jobfile test-mcp-race test-mcp-syntax
.PHONY: smoke-mcp-contract smoke-transcript-fidelity smoke-transcript-cross smoke-continue-live
.PHONY: smoke-mcp-presets smoke-presets smoke-mcp-effort-live smoke-mcp-isolation-live

# Schema validation reuses the protocol module's ajv; empty when the module dir is unresolvable, and the smoke reports it skipped.
AGENTX_PROTOCOL_DIR ?= $(shell go list -m -f '{{.Dir}}' github.com/ArcheMind/agentx-protocol 2>/dev/null)

fmt:
	gofmt -w cmd internal

check:
	go vet ./...
	sh -n install.sh test/smoke/install-clean.sh test/smoke/record-hook.sh test/smoke/hooks-live.sh test/smoke/skills-clean.sh test/smoke/skills-live.sh

test:
	go test ./...

test-subagents:
	go test ./internal/subagents ./internal/app

smoke-subagents: smoke-mcp-jobfile

smoke-subagents-live: smoke-mcp-live smoke-mcp-jobfile

audit:
	go test ./internal/app ./internal/drivers -run 'Consistency|Capabilities|Grammar|Terminology|Structured'

build:
	mkdir -p bin
	go build -trimpath -o bin/sm ./cmd/sm

install: build
	install -m 0755 bin/sm "$${SM_INSTALL_DIR:-$$HOME/.local/bin}/sm"

smoke-mcp-jobfile: build
	node test/smoke/mcp-jobfile.mjs "$(CURDIR)/bin/sm"

smoke-mcp-live: build
	node test/smoke/mcp-live.mjs "$(CURDIR)/bin/sm"

smoke-mcp-management: build
	node test/smoke/mcp-management.mjs "$(CURDIR)/bin/sm"

smoke-mcp-contract: build
	node test/smoke/mcp-contract.mjs "$(CURDIR)/bin/sm"

smoke-mcp-presets: build
	node test/smoke/mcp-presets.mjs "$(CURDIR)/bin/sm"

smoke-presets: build
	node test/smoke/presets.mjs "$(CURDIR)/bin/sm"

smoke-transcript-fidelity: build
	node test/smoke/transcript-fidelity.mjs "$(CURDIR)/bin/sm" "$(AGENTX_PROTOCOL_DIR)"

smoke-transcript-cross: build
	node test/smoke/transcript-cross.mjs "$(CURDIR)/bin/sm"

smoke-continue-live: build
	node test/smoke/continue-roundtrip-live.mjs "$(CURDIR)/bin/sm"

smoke-mcp-effort-live: build
	node test/smoke/mcp-roles-effort-live.mjs "$(CURDIR)/bin/sm"

smoke-mcp-isolation-live: build
	node test/smoke/mcp-isolation-live.mjs "$(CURDIR)/bin/sm"

test-mcp-race:
	go test -race ./internal/subagents ./internal/app

test-mcp-syntax:
	for file in test/smoke/mcp-*.mjs test/smoke/presets.mjs; do node --check "$$file" || exit 1; done
	python3 -c 'import ast; from pathlib import Path; ast.parse(Path("test/smoke/pty-host.py").read_text())'

smoke: build
	./bin/sm version
	./bin/sm --yaml status >/dev/null
	./bin/sm --json agent list >/dev/null
	./bin/sm --yaml agent list >/dev/null
	./bin/sm --json skill list >/dev/null
	./bin/sm --json hook list >/dev/null
	./bin/sm agent install codex --dry-run >/dev/null
	./bin/sm --yaml auth login codex --dry-run >/dev/null
	./bin/sm agent run codex --model smoke-test --dry-run >/dev/null
	echo '{"summary":{"id":"test","provider":"claude","message_count":0},"messages":[]}' | ./bin/sm convert to claude >/dev/null
	node test/smoke/transcript-fidelity.mjs "$(CURDIR)/bin/sm" "$(AGENTX_PROTOCOL_DIR)" >/dev/null
	node test/smoke/transcript-cross.mjs "$(CURDIR)/bin/sm" >/dev/null

smoke-live: build
	./bin/sm --json agent models codex >/dev/null
	./bin/sm --json agent models opencode >/dev/null
	./bin/sm --json agent models pi >/dev/null
	SM_LOG=debug ./bin/sm --json agent list >/dev/null 2>&1

smoke-clean:
	docker build --no-cache --target agents-clean -f test/smoke/Dockerfile -t agentx-smoke-clean .

smoke-hooks-live: build
	sh test/smoke/hooks-live.sh "$(CURDIR)/bin/sm"

smoke-skills-clean:
	docker build --no-cache --target skills-clean -f test/smoke/Dockerfile -t agentx-smoke-skills-clean .

smoke-skills-live: build
	sh test/smoke/skills-live.sh "$(CURDIR)/bin/sm"

verify: fmt check test audit smoke

clean:
	rm -f bin/sm

.PHONY: smoke-claude-models
smoke-claude-models: build
	./bin/sm --json agent models claude
	./bin/sm status
