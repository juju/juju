// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package subnet discovers the CIDRs from which Kubernetes pods
// receive IP addresses.
//
// Pod-subnet discovery combines CNI-specific strategies for Calico, Cilium,
// OVN-Kubernetes, and Kube-OVN with a generic node-based strategy for networks
// such as kube-router. Discovery reads Kubernetes resources using the model's
// credential and does not modify the cluster.
//
// See github.com/juju/juju/internal/provider/kubernetes for the Kubernetes
// provider integration. See github.com/juju/juju/core/network for the subnet
// types returned to callers. See the sections below for discovery behavior and
// package-wide limitations.
//
// # Discovery model
//
// Discovery uses an ordered, first-match chain. CNI-specific strategies run
// before the generic node strategy because node PodCIDRs are not authoritative
// for every CNI IPAM mode. Each strategy reads the source of truth for its CNI,
// and the first confident, non-empty result stops the chain. Results from
// different CNIs are never combined.
//
// Candidate CIDRs are masked to their canonical network, deduplicated, and
// sorted. More-specific networks are not collapsed into larger networks.
//
// # Failure behavior
//
// Discovery never returns a Kubernetes read error to its caller. An absent or
// unavailable source lets the next strategy run. When continuing could produce
// a misleading result -- for example, after access is denied to an identified
// CNI source -- discovery returns the IPv4 and IPv6 universe CIDRs. This safe
// fallback stops the chain before the generic node strategy can claim a range
// that the active CNI does not use.
//
// # Limitations
//
// Discovery assumes one standard CNI per cluster. Mixed-CNI and other
// non-standard topologies are not supported. Discovery does not identify stale
// CRD definitions or objects left by a removed CNI; those resources can affect
// which strategy matches before the active CNI is reached.
//
// Discovery covers only the primary pod network. Secondary networks, user
// defined networks, and Windows hybrid-overlay networks are excluded. Discovery
// does not call cloud provider APIs for VPC-native pod addresses, so clusters
// without a discoverable pod CIDR use the universe-CIDR fallback.
package subnet
