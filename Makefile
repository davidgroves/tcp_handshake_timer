.PHONY: generate build test test-unit test-integration clean

CLANG ?= clang
BPF2GO ?= go run github.com/cilium/ebpf/cmd/bpf2go

generate:
	cd internal/ebpf && go generate ./...

build: generate
	go build -tags linux -o bin/thtimer ./cmd/thtimer

test: test-unit

test-unit:
	go test -tags linux ./...

test-integration:
	sudo go test -tags "linux integration" ./test/integration/...

clean:
	rm -rf bin internal/ebpf/handshake_*bpf*.go internal/ebpf/handshake_*bpf*.o
