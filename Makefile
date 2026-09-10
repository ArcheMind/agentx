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
	./bin/ax --json list >/dev/null
	./bin/ax --yaml list >/dev/null
	./bin/ax install codex --dry-run >/dev/null
	./bin/ax --yaml auth login codex --dry-run >/dev/null
	./bin/ax run codex --model smoke-test --dry-run >/dev/null
	./bin/ax --json session providers >/dev/null

smoke-live: build
	./bin/ax --json models codex >/dev/null
	./bin/ax --json models opencode >/dev/null
	./bin/ax --json models pi >/dev/null
	AX_LOG=debug ./bin/ax --json list >/dev/null 2>&1

verify: fmt check test smoke

clean:
	rm -f bin/ax
