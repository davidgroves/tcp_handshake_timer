package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -tags linux -target amd64 Handshake ../../bpf/handshake.bpf.c -- -I/usr/include -I/usr/include/x86_64-linux-gnu
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -tags linux -target arm64 Handshake ../../bpf/handshake.bpf.c -- -I/usr/include -I/usr/include/aarch64-linux-gnu -I/usr/aarch64-linux-gnu/include
