.PHONY: fmt check test test-subagents smoke-subagents smoke-subagents-live audit build install smoke smoke-live smoke-clean smoke-hooks-live smoke-skills-clean smoke-skills-live verify clean
.PHONY: smoke-mcp-management smoke-mcp-live smoke-mcp-channel smoke-mcp-host-live verify-mcp-host-evidence test-mcp-race test-mcp-syntax
.PHONY: smoke-mcp-contract
.PHONY: smoke-mcp-profiles smoke-profiles smoke-mcp-policy-live smoke-mcp-effort-live smoke-mcp-isolation-live

fmt:
	gofmt -w cmd internal

check:
	go vet ./...
	sh -n install.sh test/smoke/install-clean.sh test/smoke/record-hook.sh test/smoke/hooks-live.sh test/smoke/skills-clean.sh test/smoke/skills-live.sh

test:
	go test ./...

test-subagents:
	go test ./internal/subagents ./internal/app

smoke-subagents: smoke-mcp-channel

smoke-subagents-live: smoke-mcp-live smoke-mcp-channel

audit:
	go test ./internal/app ./internal/drivers -run 'Consistency|Capabilities|Grammar|Terminology|Structured'

build:
	mkdir -p bin
	go build -trimpath -o bin/ax ./cmd/ax

install: build
	install -m 0755 bin/ax "$${AX_INSTALL_DIR:-$$HOME/.local/bin}/ax"

smoke-mcp-channel: build
	node test/smoke/mcp-channel.mjs "$(CURDIR)/bin/ax"

smoke-mcp-host-live: build
	node test/smoke/mcp-channel.mjs "$(CURDIR)/bin/ax" --live

verify-mcp-host-evidence:
	node test/smoke/mcp-host-evidence.mjs "$(MCP_EVIDENCE_DIR)" "$(MCP_TRANSCRIPT)" $(if $(filter historical,$(MCP_EVIDENCE_FORMAT)),--historical,)

smoke-mcp-live: build
	node test/smoke/mcp-live.mjs "$(CURDIR)/bin/ax"

smoke-mcp-management: build
	node test/smoke/mcp-management.mjs "$(CURDIR)/bin/ax"

smoke-mcp-contract: build
	node test/smoke/mcp-contract.mjs "$(CURDIR)/bin/ax"

smoke-mcp-profiles: build
	node test/smoke/mcp-profiles.mjs "$(CURDIR)/bin/ax"

smoke-profiles: build
	node test/smoke/profiles.mjs "$(CURDIR)/bin/ax"

smoke-mcp-policy-live: build
	node test/smoke/mcp-policy-live.mjs "$(CURDIR)/bin/ax"

smoke-mcp-effort-live: build
	node test/smoke/mcp-roles-effort-live.mjs "$(CURDIR)/bin/ax"

smoke-mcp-isolation-live: build
	node test/smoke/mcp-isolation-live.mjs "$(CURDIR)/bin/ax"

test-mcp-race:
	go test -race ./internal/subagents ./internal/app

test-mcp-syntax:
	for file in test/smoke/mcp-*.mjs test/smoke/profiles.mjs; do node --check "$$file" || exit 1; done
	python3 -c 'import ast; from pathlib import Path; ast.parse(Path("test/smoke/pty-host.py").read_text())'

smoke: build
	./bin/ax version
	./bin/ax --yaml status >/dev/null
	./bin/ax --json agent list >/dev/null
	./bin/ax --yaml agent list >/dev/null
	./bin/ax --json skill list >/dev/null
	./bin/ax --json hook list >/dev/null
	./bin/ax agent install codex --dry-run >/dev/null
	./bin/ax --yaml auth login codex --dry-run >/dev/null
	./bin/ax agent run codex --model smoke-test --dry-run >/dev/null

smoke-live: build
	./bin/ax --json agent models codex >/dev/null
	./bin/ax --json agent models opencode >/dev/null
	./bin/ax --json agent models pi >/dev/null
	AX_LOG=debug ./bin/ax --json agent list >/dev/null 2>&1

smoke-clean:
	docker build --no-cache --target agents-clean -f test/smoke/Dockerfile -t agentx-smoke-clean .

smoke-hooks-live: build
	sh test/smoke/hooks-live.sh "$(CURDIR)/bin/ax"

smoke-skills-clean:
	docker build --no-cache --target skills-clean -f test/smoke/Dockerfile -t agentx-smoke-skills-clean .

smoke-skills-live: build
	sh test/smoke/skills-live.sh "$(CURDIR)/bin/ax"

verify: fmt check test audit smoke

clean:
	rm -f bin/ax

.PHONY: smoke-claude-models
smoke-claude-models: build
	./bin/ax --json agent models claude
	./bin/ax status
