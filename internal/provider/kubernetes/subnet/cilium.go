// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const ciliumDiscovererName = "cilium"

// ciliumDiscoverer reads Cilium's pod CIDRs from the source dictated by the
// IPAM mode, detected from the kube-system/cilium-config ConfigMap ipam key.
type ciliumDiscoverer struct{}

func (ciliumDiscoverer) Name() string { return ciliumDiscovererName }

// Discover detects Cilium from cilium-config and selects the authoritative
// source for its configured IPAM mode.
func (ciliumDiscoverer) Discover(ctx context.Context, clients Clients) ([]string, error) {
	if clients.Typed == nil {
		return nil, nil
	}
	cm, err := clients.Typed.CoreV1().ConfigMaps("kube-system").Get(ctx, "cilium-config", metav1.GetOptions{})
	if err != nil {
		if deniedReadingResource(ctx, ciliumDiscovererName, "the kube-system/cilium-config ConfigMap", err) {
			return fallbackCIDRs, nil
		}
		return nil, nil
	}

	mode := strings.TrimSpace(cm.Data["ipam"])
	if mode == "" {
		// cluster-pool is the default mode.
		mode = "cluster-pool"
	}

	switch mode {
	case "kubernetes":
		// kcm-allocated: node.Spec.PodCIDR(s) is authoritative in this mode.
		cidrs, denied := nodeSpecCIDRs(ctx, clients, ciliumDiscovererName)
		if denied {
			return fallbackCIDRs, nil
		}
		return cidrs, nil
	case "cluster-pool":
		return ciliumClusterPoolCIDRs(ctx, clients, cm.Data)
	case "multi-pool":
		return ciliumMultiPoolCIDRs(ctx, clients)
	case "eni", "azure", "alibabacloud", "crd":
		// VPC-native IPs (eni/azure/alibabacloud) or individual IPs (crd):
		// no pod CIDR to discover.
		logger.Debugf(ctx, "cilium pod-subnet discovery: ipam mode %q has no pod CIDR", mode)
		// Cilium is present, so stop the chain here. Returning nil would let
		// generic node discovery treat node.Spec.PodCIDR as authoritative even
		// though these modes do not allocate pod addresses from that range.
		return fallbackCIDRs, nil
	default:
		logger.Debugf(ctx, "cilium pod-subnet discovery: unrecognised ipam mode %q", mode)
		return nil, nil
	}
}

// ciliumClusterPoolCIDRs returns the cluster-wide cluster-pool CIDRs from the
// cilium-config ConfigMap (preferred, core-only), falling back to the per-node
// CiliumNode podCIDRs when the ConfigMap keys are absent.
func ciliumClusterPoolCIDRs(ctx context.Context, clients Clients, data map[string]string) ([]string, error) {
	var out []string
	out = append(out, splitCIDRCandidates(data["cluster-pool-ipv4-cidr"])...)
	out = append(out, splitCIDRCandidates(data["cluster-pool-ipv6-cidr"])...)
	if len(out) > 0 {
		return out, nil
	}
	cidrs, denied := ciliumNodePodCIDRs(ctx, clients)
	if denied {
		return fallbackCIDRs, nil
	}
	return cidrs, nil
}

// ciliumNodePodCIDRs lists CiliumNode objects and unions their
// spec.ipam.podCIDRs (operator-allocated per-node ranges).
func ciliumNodePodCIDRs(ctx context.Context, clients Clients) (cidrs []string, denied bool) {
	if clients.APIExtensions == nil || clients.Dynamic == nil {
		return nil, false
	}
	gvr, found, denied := firstServedCRD(ctx, clients, ciliumDiscovererName, "ciliumnodes.cilium.io")
	if denied {
		return nil, true
	}
	if !found {
		return nil, false
	}
	list, err := clients.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, deniedReadingResource(ctx, ciliumDiscovererName, "CiliumNode resources", err)
	}
	for i := range list.Items {
		item := list.Items[i]
		podCIDRs, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "ipam", "podCIDRs")
		cidrs = append(cidrs, podCIDRs...)
	}
	return cidrs, false
}

// ciliumMultiPoolCIDRs returns the multi-pool CIDRs from CiliumPodIPPool
// objects (preferred), falling back to the per-node CiliumNode allocated pool
// CIDRs.
func ciliumMultiPoolCIDRs(ctx context.Context, clients Clients) ([]string, error) {
	if clients.APIExtensions == nil || clients.Dynamic == nil {
		return nil, nil
	}
	gvr, found, denied := firstServedCRD(ctx, clients, ciliumDiscovererName, "ciliumpodippools.cilium.io")
	if denied {
		return fallbackCIDRs, nil
	}
	if found {
		cidrs, denied := ciliumPodIPPoolCIDRs(ctx, clients, gvr)
		if denied {
			return fallbackCIDRs, nil
		}
		if len(cidrs) > 0 {
			return cidrs, nil
		}
	}
	cidrs, denied := ciliumNodeAllocatedCIDRs(ctx, clients)
	if denied {
		return fallbackCIDRs, nil
	}
	return cidrs, nil
}

// ciliumPodIPPoolCIDRs lists CiliumPodIPPool objects and unions their
// spec.ipv4.cidrs / spec.ipv6.cidrs.
func ciliumPodIPPoolCIDRs(ctx context.Context, clients Clients, gvr schema.GroupVersionResource) (cidrs []string, denied bool) {
	list, err := clients.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, deniedReadingResource(ctx, ciliumDiscovererName, "CiliumPodIPPool resources", err)
	}
	for i := range list.Items {
		item := list.Items[i]
		ipv4, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "ipv4", "cidrs")
		ipv6, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "ipv6", "cidrs")
		cidrs = append(cidrs, ipv4...)
		cidrs = append(cidrs, ipv6...)
	}
	return cidrs, false
}

// ciliumNodeAllocatedCIDRs lists CiliumNode objects and unions their
// spec.ipam.pools.allocated[].cidrs (multi-pool per-node allocations).
func ciliumNodeAllocatedCIDRs(ctx context.Context, clients Clients) (cidrs []string, denied bool) {
	gvr, found, denied := firstServedCRD(ctx, clients, ciliumDiscovererName, "ciliumnodes.cilium.io")
	if denied {
		return nil, true
	}
	if !found {
		return nil, false
	}
	list, err := clients.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, deniedReadingResource(ctx, ciliumDiscovererName, "CiliumNode resources", err)
	}
	for i := range list.Items {
		item := list.Items[i]
		allocated, _, _ := unstructured.NestedSlice(item.Object, "spec", "ipam", "pools", "allocated")
		for _, entry := range allocated {
			m, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			rawCIDRs, ok := m["cidrs"].([]any)
			if !ok {
				continue
			}
			for _, c := range rawCIDRs {
				if s, ok := c.(string); ok && s != "" {
					cidrs = append(cidrs, s)
				}
			}
		}
	}
	return cidrs, false
}
