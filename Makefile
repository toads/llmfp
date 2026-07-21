.PHONY: build check clean fmt integration test

BIN ?= llmfp

build:
	go build -o $(BIN) ./cmd/llmfp

fmt:
	gofmt -w $$(find cmd internal -name '*.go' -type f)

test:
	go test ./...

integration:
	LLMFP_INTEGRATION=1 go test ./internal/attribution ./internal/run -count=1 -v

check:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test -race ./...
	go build ./cmd/llmfp

clean:
	rm -f $(BIN)

