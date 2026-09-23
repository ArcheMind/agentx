.PHONY: fmt check test audit build smoke smoke-live smoke-clean smoke-hooks-live verify clean

fmt:
	gofmt -w cmd internal

check:
	go vet ./...
	sh -n install.sh test/smoke/install-clean.sh test/smoke/record-hook.sh test/smoke/hooks-live.sh

test:
	go test ./...

audit:
	go test ./internal/app ./internal/drivers -run 'Consistency|Capabilities|Grammar|Terminology|Structured'

build:
	mkdir -p bin
	go build -trimpath -o bin/ax ./cmd/ax

smoke: build
	./bin/ax version
	./bin/ax --yaml list >/dev/null
	./bin/ax --json agent list >/dev/null
	./bin/ax --yaml agent list >/dev/null
	./bin/ax --json skill list >/dev/null
	./bin/ax --json hook list >/dev/null
	./bin/ax agent install codex --dry-run >/dev/null
	./bin/ax --yaml auth login codex --dry-run >/dev/null
	./bin/ax agent run codex --model smoke-test --dry-run >/dev/null
	echo '{"summary":{"id":"test","provider":"claude","message_count":0},"messages":[]}' | ./bin/ax convert --to claude >/dev/null

smoke-live: build
	./bin/ax --json agent models codex >/dev/null
	./bin/ax --json agent models opencode >/dev/null
	./bin/ax --json agent models pi >/dev/null
	AX_LOG=debug ./bin/ax --json agent list >/dev/null 2>&1

smoke-clean:
	docker build --no-cache -f test/smoke/Dockerfile -t agentx-smoke-clean .

smoke-hooks-live: build
	sh test/smoke/hooks-live.sh "$(CURDIR)/bin/ax"

verify: fmt check test audit smoke

clean:
	rm -f bin/ax
