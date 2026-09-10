.PHONY: fmt check test audit build smoke smoke-live verify clean

fmt:
	gofmt -w cmd internal

check:
	go vet ./...

test:
	go test ./...

audit:
	go test ./internal/app ./internal/drivers -run 'Consistency|Capabilities|Grammar|Terminology|Structured'

build:
	mkdir -p bin
	go build -trimpath -o bin/ax ./cmd/ax

smoke: build
	./bin/ax version
	./bin/ax --json agent list >/dev/null
	./bin/ax --yaml agent list >/dev/null
	./bin/ax agent install codex --dry-run >/dev/null
	./bin/ax --yaml auth login codex --dry-run >/dev/null
	./bin/ax agent run codex --model smoke-test --dry-run >/dev/null
	./bin/ax --json session providers >/dev/null

smoke-live: build
	./bin/ax --json agent models codex >/dev/null
	./bin/ax --json agent models opencode >/dev/null
	./bin/ax --json agent models pi >/dev/null
	AX_LOG=debug ./bin/ax --json agent list >/dev/null 2>&1

verify: fmt check test audit smoke

clean:
	rm -f bin/ax
