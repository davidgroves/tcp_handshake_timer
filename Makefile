.PHONY: generate swagger-ui build test test-unit test-integration clean

CLANG ?= clang
BPF2GO ?= go run github.com/cilium/ebpf/cmd/bpf2go
SWAGGER_UI_VERSION ?= 5.18.3

swagger-ui:
	SWAGGER_UI_VERSION=$(SWAGGER_UI_VERSION) ./scripts/fetch-swagger-ui.sh

generate: swagger-ui
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
	rm -f internal/api/static/swagger-ui/swagger-ui.css internal/api/static/swagger-ui/swagger-ui-bundle.js
