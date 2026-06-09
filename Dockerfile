
# thtimer attaches eBPF TC/TCX programs to host network interfaces. The container
# must use the host network namespace and Linux capabilities (see README Docker
# section). Runtime image is scratch + statically linked binary only; config is
# mounted at run time.

FROM golang:bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    clang \
    curl \
    llvm \
    libbpf-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETARCH
ARG SWAGGER_UI_VERSION=5.18.3
RUN SWAGGER_UI_VERSION=${SWAGGER_UI_VERSION} ./scripts/fetch-swagger-ui.sh
RUN cd internal/ebpf && go generate ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -tags linux -ldflags="-s -w" -o /out/thtimer ./cmd/thtimer

FROM scratch

COPY --from=builder /out/thtimer /thtimer

ENTRYPOINT ["/thtimer"]
CMD ["-config", "/config/config.yaml"]
