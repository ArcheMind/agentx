.PHONY: fmt check test build smoke smoke-live verify clean

fmt:
	gofmt -w cmd internal

check:
	go vet ./...

test:
	go test ./...

build:
	mkdir -p bin
	go build -trimpath -o bin/ax ./cmd/ax

smoke: build
	./bin/ax version
	./bin/ax list --json >/dev/null
	./bin/ax install codex --dry-run >/dev/null
	./bin/ax run codex --model smoke-test --dry-run >/dev/null

smoke-live: build
	./bin/ax models codex --json >/dev/null
	./bin/ax models opencode --json >/dev/null
	./bin/ax models pi --json >/dev/null
	AX_LOG=debug ./bin/ax list --json >/dev/null 2>&1

verify: fmt check test smoke

clean:
	rm -f bin/ax
