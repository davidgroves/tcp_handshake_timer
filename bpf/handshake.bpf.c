//go:build ignore

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/in.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/pkt_cls.h>
#include <linux/tcp.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

char __license[] SEC("license") = "Dual MIT/GPL";

#define AF_INET  2
#define AF_INET6 10

#define STAT_SYNACK_OBSERVED 0
#define STAT_SYNACK_SAMPLED  1
#define STAT_ACK_MATCHED     2
#define STAT_INCOMPLETE      3
#define STAT_RINGBUF_DROP    4
#define STAT_PARSE_ERROR     5
#define STAT_PORT_SKIP       6
#define STAT_MAX             7

struct flow_key {
	__u8  family;
	__u8  _pad[3];
	__u8  client_ip[16];
	__u8  server_ip[16];
	__u16 client_port;
	__u16 server_port;
};

struct flow_state {
	__u64 ts_ns;
	__u32 server_isn;
	__u32 _pad;
};

struct thtimer_config {
	__u32 sample_rate_pct;
	__u32 port_filter_enabled;
	__u32 prefix_v4;
	__u32 prefix_v6;
};

struct measurement_event {
	__u8  family;
	__u8  _pad[3];
	__u8  client_ip[16];
	__u8  server_ip[16];
	__u16 client_port;
	__u16 server_port;
	__u64 rtt_ns;
	__u64 ts_ns;
};

struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 65536);
	__type(key, struct flow_key);
	__type(value, struct flow_state);
} flows SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 256);
	__type(key, __u16);
	__type(value, __u8);
} ports SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, struct thtimer_config);
} config_map SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 24);
} events SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, STAT_MAX);
	__type(key, __u32);
	__type(value, __u64);
} stats SEC(".maps");

static __always_inline void stat_inc(__u32 idx)
{
	__u64 *val = bpf_map_lookup_elem(&stats, &idx);
	if (val)
		__sync_fetch_and_add(val, 1);
}

static __always_inline struct thtimer_config *load_config(void)
{
	__u32 key = 0;
	return bpf_map_lookup_elem(&config_map, &key);
}

static __always_inline int port_allowed(__u16 port, struct thtimer_config *cfg)
{
	if (!cfg || !cfg->port_filter_enabled)
		return 1;

	__u16 key = port;
	return bpf_map_lookup_elem(&ports, &key) != NULL;
}

static __always_inline void ip4_to_bytes(__be32 addr, __u8 out[16])
{
	__builtin_memset(out, 0, 16);
	__builtin_memcpy(out, &addr, 4);
}

static __always_inline void ip6_copy(__u8 out[16], const __u8 in[16])
{
	__builtin_memcpy(out, in, 16);
}

static __always_inline int sampled(struct thtimer_config *cfg)
{
	if (!cfg || cfg->sample_rate_pct >= 100)
		return 1;
	if (cfg->sample_rate_pct == 0)
		return 0;
	return (bpf_get_prandom_u32() % 100) < cfg->sample_rate_pct;
}

static __always_inline void emit_measurement(struct flow_key *key, __u64 rtt_ns, __u64 ts_ns)
{
	struct measurement_event *evt;

	evt = bpf_ringbuf_reserve(&events, sizeof(*evt), 0);
	if (!evt) {
		stat_inc(STAT_RINGBUF_DROP);
		return;
	}

	evt->family = key->family;
	__builtin_memcpy(evt->client_ip, key->client_ip, 16);
	__builtin_memcpy(evt->server_ip, key->server_ip, 16);
	evt->client_port = key->client_port;
	evt->server_port = key->server_port;
	evt->rtt_ns = rtt_ns;
	evt->ts_ns = ts_ns;
	bpf_ringbuf_submit(evt, 0);
}

static __always_inline void *parse_eth(void *data, void *data_end, __u16 *eth_proto)
{
	struct ethhdr *eth = data;

	if ((void *)(eth + 1) > data_end)
		return NULL;

	*eth_proto = bpf_ntohs(eth->h_proto);
	return (void *)(eth + 1);
}

static __always_inline int parse_l2_l3(void *data, void *data_end, __u16 *eth_proto, void **l3_out)
{
	struct ethhdr *eth = data;

	if ((void *)(eth + 1) <= data_end) {
		__u16 proto = bpf_ntohs(eth->h_proto);
		if (proto == ETH_P_IP || proto == ETH_P_IPV6) {
			*eth_proto = proto;
			*l3_out = (void *)(eth + 1);
			return 0;
		}
	}

	struct iphdr *ip = data;
	if ((void *)(ip + 1) <= data_end && ip->version == 4) {
		*eth_proto = ETH_P_IP;
		*l3_out = data;
		return 0;
	}

	struct ipv6hdr *ip6 = data;
	if ((void *)(ip6 + 1) <= data_end && ip6->version == 6) {
		*eth_proto = ETH_P_IPV6;
		*l3_out = data;
		return 0;
	}

	return -1;
}

static __always_inline void *parse_ipv4(void *data, void *data_end, struct iphdr **ip_out)
{
	struct iphdr *ip = data;

	if ((void *)(ip + 1) > data_end)
		return NULL;
	if (ip->ihl < 5)
		return NULL;
	if (ip->version != 4)
		return NULL;
	if (ip->protocol != IPPROTO_TCP)
		return NULL;

	*ip_out = ip;
	return (void *)ip + (ip->ihl * 4);
}

static __always_inline void *skip_ipv6_ext(void *cursor, void *data_end, __u8 *nexthdr)
{
	#pragma unroll
	for (int i = 0; i < 4; i++) {
		if (*nexthdr == IPPROTO_TCP)
			return cursor;
		if (*nexthdr == 0 || *nexthdr == 43 || *nexthdr == 44 || *nexthdr == 60) {
			struct ipv6_opt_hdr *opt = cursor;
			if ((void *)(opt + 1) > data_end)
				return NULL;
			__u32 len = (opt->hdrlen + 1) * 8;
			*nexthdr = opt->nexthdr;
			cursor += len;
			continue;
		}
		return NULL;
	}
	return NULL;
}

static __always_inline void *parse_ipv6(void *data, void *data_end, struct ipv6hdr **ip6_out)
{
	struct ipv6hdr *ip6 = data;
	__u8 nexthdr;

	if ((void *)(ip6 + 1) > data_end)
		return NULL;
	if (ip6->version != 6)
		return NULL;

	nexthdr = ip6->nexthdr;
	void *cursor = (void *)(ip6 + 1);
	void *tcp_start = skip_ipv6_ext(cursor, data_end, &nexthdr);
	if (!tcp_start || nexthdr != IPPROTO_TCP)
		return NULL;

	*ip6_out = ip6;
	return tcp_start;
}

static __always_inline struct tcphdr *parse_tcp(void *data, void *data_end)
{
	struct tcphdr *tcp = data;

	if ((void *)(tcp + 1) > data_end)
		return NULL;
	if (tcp->doff < 5)
		return NULL;
	if ((void *)tcp + (tcp->doff * 4) > data_end)
		return NULL;
	return tcp;
}

static __always_inline int handle_egress(struct __sk_buff *skb)
{
	struct thtimer_config *cfg = load_config();
	void *data = (void *)(long)skb->data;
	void *data_end = (void *)(long)skb->data_end;
	__u16 eth_proto;
	void *l3;
	struct iphdr *ip4 = NULL;
	struct ipv6hdr *ip6 = NULL;
	struct tcphdr *tcp;
	struct flow_key key = {};
	struct flow_state state = {};
	__u32 seq;

	if (parse_l2_l3(data, data_end, &eth_proto, &l3) < 0) {
		stat_inc(STAT_PARSE_ERROR);
		return TC_ACT_OK;
	}

	if (eth_proto == ETH_P_IP) {
		l3 = parse_ipv4(l3, data_end, &ip4);
		if (!l3 || !ip4) {
			stat_inc(STAT_PARSE_ERROR);
			return TC_ACT_OK;
		}
		key.family = AF_INET;
		ip4_to_bytes(ip4->daddr, key.client_ip);
		ip4_to_bytes(ip4->saddr, key.server_ip);
	} else if (eth_proto == ETH_P_IPV6) {
		l3 = parse_ipv6(l3, data_end, &ip6);
		if (!l3 || !ip6) {
			stat_inc(STAT_PARSE_ERROR);
			return TC_ACT_OK;
		}
		key.family = AF_INET6;
		ip6_copy(key.client_ip, (const __u8 *)ip6->daddr.s6_addr);
		ip6_copy(key.server_ip, (const __u8 *)ip6->saddr.s6_addr);
	} else {
		return TC_ACT_OK;
	}

	tcp = parse_tcp(l3, data_end);
	if (!tcp)
		return TC_ACT_OK;

	key.client_port = bpf_ntohs(tcp->dest);
	key.server_port = bpf_ntohs(tcp->source);

	if (!port_allowed(key.server_port, cfg)) {
		stat_inc(STAT_PORT_SKIP);
		return TC_ACT_OK;
	}

	if (!tcp->syn || !tcp->ack || tcp->rst)
		return TC_ACT_OK;

	stat_inc(STAT_SYNACK_OBSERVED);

	if (!sampled(cfg))
		return TC_ACT_OK;

	if (bpf_map_lookup_elem(&flows, &key))
		return TC_ACT_OK;

	stat_inc(STAT_SYNACK_SAMPLED);
	seq = bpf_ntohl(tcp->seq);
	state.ts_ns = bpf_ktime_get_ns();
	state.server_isn = seq;
	bpf_map_update_elem(&flows, &key, &state, BPF_NOEXIST);
	return TC_ACT_OK;
}

static __always_inline int handle_ingress(struct __sk_buff *skb)
{
	struct thtimer_config *cfg = load_config();
	void *data = (void *)(long)skb->data;
	void *data_end = (void *)(long)skb->data_end;
	__u16 eth_proto;
	void *l3;
	struct iphdr *ip4 = NULL;
	struct ipv6hdr *ip6 = NULL;
	struct tcphdr *tcp;
	struct flow_key key = {};
	struct flow_state *st;
	__u64 now;
	__u64 rtt;

	if (parse_l2_l3(data, data_end, &eth_proto, &l3) < 0) {
		stat_inc(STAT_PARSE_ERROR);
		return TC_ACT_OK;
	}

	if (eth_proto == ETH_P_IP) {
		l3 = parse_ipv4(l3, data_end, &ip4);
		if (!l3 || !ip4) {
			stat_inc(STAT_PARSE_ERROR);
			return TC_ACT_OK;
		}
		key.family = AF_INET;
		ip4_to_bytes(ip4->saddr, key.client_ip);
		ip4_to_bytes(ip4->daddr, key.server_ip);
	} else if (eth_proto == ETH_P_IPV6) {
		l3 = parse_ipv6(l3, data_end, &ip6);
		if (!l3 || !ip6) {
			stat_inc(STAT_PARSE_ERROR);
			return TC_ACT_OK;
		}
		key.family = AF_INET6;
		ip6_copy(key.client_ip, (const __u8 *)ip6->saddr.s6_addr);
		ip6_copy(key.server_ip, (const __u8 *)ip6->daddr.s6_addr);
	} else {
		return TC_ACT_OK;
	}

	tcp = parse_tcp(l3, data_end);
	if (!tcp)
		return TC_ACT_OK;

	key.client_port = bpf_ntohs(tcp->source);
	key.server_port = bpf_ntohs(tcp->dest);

	if (!port_allowed(key.server_port, cfg)) {
		stat_inc(STAT_PORT_SKIP);
		return TC_ACT_OK;
	}

	if (tcp->syn || tcp->rst || !tcp->ack)
		return TC_ACT_OK;

	st = bpf_map_lookup_elem(&flows, &key);
	if (!st) {
		stat_inc(STAT_INCOMPLETE);
		return TC_ACT_OK;
	}

	if (bpf_ntohl(tcp->ack_seq) != st->server_isn + 1)
		return TC_ACT_OK;

	now = bpf_ktime_get_ns();
	rtt = now - st->ts_ns;
	stat_inc(STAT_ACK_MATCHED);
	emit_measurement(&key, rtt, now);
	bpf_map_delete_elem(&flows, &key);
	return TC_ACT_OK;
}

SEC("tc")
int thtimer_ingress(struct __sk_buff *skb)
{
	return handle_ingress(skb);
}

SEC("tc")
int thtimer_egress(struct __sk_buff *skb)
{
	return handle_egress(skb);
}
