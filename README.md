# thtimer

Passive TCP handshake RTT measurement for DNS/TCP servers using eBPF. The program attaches TC/TCX programs on configured interfaces, tracks SYN-ACK egress timestamps, and measures the interval until the client's completing ACK arrives on ingress.

## Features

- eBPF TCX attach with clsact fallback
- Configurable interfaces, TCP ports, and in-kernel sampling rate
- IPv4 and IPv6 support
- Prometheus metrics at `/metrics`
- Sampled JSON logs to stdout
- Memory-capped in-memory store with HTTP API:
  - `GET /api/dump` — full JSON dump of retained samples
  - `GET /api/stats` — overall and per-subnet percentile stats
  - `GET /api/topn?n=100` — slowest clients
  - `GET /api/docs` — interactive API documentation (OpenAPI / Swagger UI)
  - `GET /api/openapi.yaml` — OpenAPI 3.0 specification ([source](internal/api/openapi.yaml))
  - `GET /healthz`

## Build

```bash
make generate
make build
```

Requires Go, clang, libbpf headers (`/usr/include/bpf`), and a BTF-enabled kernel (5.8+; TCX on 6.6+).

## Run

```bash
sudo ./bin/thtimer -config config.example.yaml -interfaces eth0
```

Or override settings via flags:

```bash
sudo ./bin/thtimer \
  -interfaces eth0 \
  -ports 53 \
  -sample-rate-pct 10 \
  -log-sample-pct 1 \
  -listen-addr :8080
```

## Configuration

See [`config.example.yaml`](config.example.yaml). Flags override YAML values.

| Setting | Description |
|---------|-------------|
| `interfaces` | NICs to attach eBPF programs |
| `ports` | TCP ports to monitor (empty = all ports) |
| `sample_rate_pct` | Percentage of SYN-ACK flows tracked in kernel |
| `log_sample_pct` | Percentage of completed measurements logged |
| `max_memory` / `max_samples` | Userspace state retention limits |
| `subnet_prefix_v4/v6` | Client subnet aggregation prefix lengths |

## Metrics

- `thtimer_measurements_made_total`
- `thtimer_measurements_logged_total`
- `thtimer_handshake_rtt_seconds` (histogram)
- `thtimer_synacks_observed_total`
- `thtimer_incomplete_handshakes_total`
- `thtimer_ringbuf_events_dropped_total`
- `thtimer_parse_errors_total`
- `thtimer_flows_in_flight`
- `thtimer_state_samples`
- `thtimer_state_memory_bytes`

## Docker

The runtime image is **`scratch` plus the statically linked binary** (~17MB).
Swagger UI, OpenAPI, and the eBPF program are embedded in the binary via
`go:embed`. Mount a config file at run time (`/config/config.yaml`).

thtimer runs eBPF programs **on the host kernel** and attaches to **host network
interfaces**. A normal bridge-networked container cannot see host NICs (`eth0`,
`wlan0`, etc.) and cannot load the required programs without extra privileges.

### Build the image

```bash
docker build -t thtimer:latest .
```

### Run (recommended)

Use **host networking** so interface names in your config match the host, and
grant the capabilities needed to load eBPF programs and attach TC/TCX hooks:

```bash
docker run --rm -it \
  --name thtimer \
  --network host \
  --cap-add BPF \
  --cap-add NET_ADMIN \
  --cap-add PERFMON \
  --cap-add SYS_ADMIN \
  --ulimit memlock=-1:-1 \
  -v "$(pwd)/config.example.yaml:/config/config.yaml:ro" \
  thtimer:latest
```

Or with Compose (see [Local development with Docker Compose](#local-development-with-docker-compose)):

```bash
docker compose up --build
```

For production, copy `config.example.yaml` and set `interfaces` to your host NIC
(e.g. `eth0`, `wlan0`). The included Compose file uses `config.docker.local.yaml`
with `lo` for local testing.

### Local development with Docker Compose

This repo includes [`docker-compose.yml`](docker-compose.yml) and
[`config.docker.local.yaml`](config.docker.local.yaml) for a minimal local setup:
attach to **`lo`**, monitor **TCP port 53**, and expose the API on **`:8080`**.

**Prerequisites**

- Docker with Compose v2
- A DNS server accepting **TCP** on `127.0.0.1:53` (systemd-resolved, BIND,
  Unbound, etc.). thtimer only observes handshakes; it does not serve DNS.
- Linux kernel with BTF and TCX (6.6+) or clsact fallback

**Start thtimer**

```bash
docker compose up --build
```

Leave it running in one terminal. Logs appear on stdout (sampled measurements as
JSON when `log_sample_pct` > 0).

**Generate TCP DNS traffic**

In another terminal (SERVFAIL is fine — the TCP handshake still completes):

```bash
# single query
dig @127.0.0.1 +tcp example.com +time=2

# burst of queries
for i in $(seq 1 20); do
  dig @127.0.0.1 +tcp example.com +time=2 +short >/dev/null
done
```

Other tools:

```bash
drill -T @127.0.0.1 example.com
kdig -T @127.0.0.1 example.com
```

**URLs — read data out**

| URL | What you get |
|-----|----------------|
| http://localhost:8080/api/docs | Interactive API docs (Swagger UI) |
| http://localhost:8080/api/dump | All retained raw measurements (JSON) |
| http://localhost:8080/api/stats | RTT percentiles overall and per subnet |
| http://localhost:8080/api/topn | Slowest clients (default top 100) |
| http://localhost:8080/api/topn?n=10 | Top 10 slowest |
| http://localhost:8080/metrics | Prometheus metrics |
| http://localhost:8080/healthz | Liveness check |
| http://localhost:8080/api/openapi.yaml | OpenAPI specification |

Quick checks from the shell:

```bash
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/api/dump | jq '.count'
curl -s http://localhost:8080/api/stats | jq '.overall'
curl -s http://localhost:8080/metrics | grep thtimer_measurements_made
```

**Stop**

```bash
docker compose down
```


### Capabilities

| Capability | Why it is needed |
|------------|------------------|
| `BPF` | Load eBPF programs and maps into the kernel |
| `NET_ADMIN` | Attach TC/TCX programs to network interfaces |
| `PERFMON` | Access kernel trace/BPF facilities on modern kernels |
| `SYS_ADMIN` | Fallback for clsact qdisc setup via netlink on some systems. Shouldn't be needed on modern kernels. |

`memlock=unlimited` (`--ulimit memlock=-1:-1`) allows the kernel to lock eBPF
map memory; thtimer also calls `RLIMIT_MEMLOCK`, but Docker defaults are often
too low.

### Important runtime notes

- **`--network host` is required** for production use on a DNS server. Without
  it, `-interfaces eth0` refers to the container's (non-existent or wrong)
  interface, not the host's.
- The HTTP API listens on the **host** at the address from `listen_addr`
  (default `:8080`) when using host networking.
- The container image contains **only the binary**; eBPF runs in the **host
  kernel** (BTF, TCX support, etc.) on the machine where `docker run` executes.
- On systems with AppArmor, you may need `--security-opt apparmor=unconfined`
  (included in `docker-compose.yml`) if BPF loading is blocked.
- `--privileged` also works but grants far more than necessary; prefer explicit
  capabilities above.

## Testing

```bash
make test-unit
sudo make test-integration
```

Integration tests create network namespaces and require root.

## DNS TC=1 use case

Configure your DNS server to set `TC=1` on a small fraction of responses so clients retry over TCP. thtimer measures the client network RTT during those TCP handshakes on the server NIC.
