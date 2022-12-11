// +build ignore

// SPDX-License-Identifier: (GPL-2.0-only OR BSD-2-Clause)
/* Copyright (c) 2020 Cloudflare */
/* Copyright (c) 2022 ArthurChiao */
/*
 * BPF socket lookup program that dispatches connections destined to a
 * configured set of open ports, to the echo service socket.
 *
 * Program expects the echo service socket to be in the `socket_map` BPF map.
 * Port is considered open when an entry for that port number exists in the
 * `port_map` BPF hashmap.
 */

#include "common.h"

char __license[] SEC("license") = "Dual BSD/GPL";

#define SK_DROP 0
#define SK_PASS 1

#define __bpf_md_ptr(type, name) \
	union { \
		type name; \
		__u64 : 64; \
	} __attribute__((aligned(8)))

/* User accessible data for SK_LOOKUP programs. Add new fields at the end. */
struct bpf_sk_lookup {
	__bpf_md_ptr(struct bpf_sock *, sk); /* Selected socket */

	__u32 family;        /* Protocol family (AF_INET, AF_INET6) */
	__u32 protocol;      /* IP protocol (IPPROTO_TCP, IPPROTO_UDP) */
	__u32 remote_ip4;    /* Network byte order */
	__u32 remote_ip6[4]; /* Network byte order */
	__u32 remote_port;   /* Network byte order */
	__u32 local_ip4;     /* Network byte order */
	__u32 local_ip6[4];  /* Network byte order */
	__u32 local_port;    /* Host byte order */
};

// Hash table for storing listening ports. Hash key is the port number.
struct bpf_map_def SEC("maps") port_map = {
	.type        = BPF_MAP_TYPE_HASH,
	.max_entries = 1024,
	.key_size    = sizeof(__u16),
	.value_size  = sizeof(__u8),
};

// Hash table for storing sockets (socket pointers)
struct bpf_map_def SEC("maps") socket_map = {
	.type        = BPF_MAP_TYPE_SOCKMAP,
	.max_entries = 1,
	.key_size    = sizeof(__u32),
	.value_size  = sizeof(__u64),
};

// Program for dispatching packets to sockets
SEC("sk_lookup/echo_dispatch")
int echo_dispatch(struct bpf_sk_lookup *ctx) {
	// Check if the given port is being served by a server
	__u16 port = ctx->local_port; // port expected to be listened on by server
	__u8 *open = bpf_map_lookup_elem(&port_map, &port);
	if (!open)          // NULL means not found,
		return SK_PASS; // we just let the packet go

	// There is a socket serving on the given port, now try to find it
	const __u32 key     = 0;
	struct bpf_sock *sk = bpf_map_lookup_elem(&socket_map, &key);
	if (!sk)            // socket not found, this is weired, user can choose
		return SK_DROP; // to drop the packet or let it go, here we just drop it

	// Dispatch the packet to the server socket
	long err = bpf_sk_assign(ctx, sk, 0);
	bpf_sk_release(sk); // Release the reference held by sk

	return err ? SK_DROP : SK_PASS;
}
