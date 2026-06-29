// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"context"
	"encoding/json"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	ovnDiscovererName = "ovn-kubernetes"

	ovnNodeSubnetsAnnotation = "k8s.ovn.org/node-subnets"
	ovnDefaultNetworkName    = "default"
)

// ovnKubernetesDiscoverer unions the three OVN-Kubernetes pod-CIDR sources (the
// per-node node-subnets annotation, the ovn-config net_cidr ConfigMap, and the
// OpenShift Network config CRD) and returns the combined set as its single
// confident result.
type ovnKubernetesDiscoverer struct{}

func (ovnKubernetesDiscoverer) Name() string { return ovnDiscovererName }

// Discover combines primary-network CIDRs from OVN-Kubernetes node annotations,
// ConfigMaps, and the OpenShift Network resource.
func (ovnKubernetesDiscoverer) Discover(ctx context.Context, clients Clients) ([]string, error) {
	if clients.Typed == nil {
		return nil, nil
	}
	var out []string

	cidrs, denied := ovnNodeSubnetCIDRs(ctx, clients)
	if denied {
		return fallbackCIDRs, nil
	}
	out = append(out, cidrs...)

	cidrs, denied = ovnConfigNetCIDRs(ctx, clients)
	if denied {
		return fallbackCIDRs, nil
	}
	out = append(out, cidrs...)

	cidrs, denied = ovnOpenShiftClusterNetworkCIDRs(ctx, clients)
	if denied {
		return fallbackCIDRs, nil
	}
	out = append(out, cidrs...)

	return out, nil
}

// ovnNodeSubnetCIDRs reads the default-network pod subnets from the
// k8s.ovn.org/node-subnets annotation on every node.
func ovnNodeSubnetCIDRs(ctx context.Context, clients Clients) (cidrs []string, denied bool) {
	nodes, err := clients.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, deniedReadingResource(ctx, ovnDiscovererName, "Nodes", err)
	}
	for _, node := range nodes.Items {
		raw := strings.TrimSpace(node.Annotations[ovnNodeSubnetsAnnotation])
		if raw == "" {
			continue
		}
		cidrs = append(cidrs, parseOVNNodeSubnets(ctx, raw)...)
	}
	return cidrs, false
}

// parseOVNNodeSubnets decodes the node-subnets annotation, handling both the
// dual-stack array schema ({"default":[...]}) and the single-stack string
// schema ({"default":"..."}). Only the default network is used.
func parseOVNNodeSubnets(ctx context.Context, raw string) []string {
	var arrays map[string][]string
	if err := json.Unmarshal([]byte(raw), &arrays); err == nil {
		return arrays[ovnDefaultNetworkName]
	}
	var singles map[string]string
	if err := json.Unmarshal([]byte(raw), &singles); err == nil {
		if v := strings.TrimSpace(singles[ovnDefaultNetworkName]); v != "" {
			return []string{v}
		}
		return nil
	}
	logger.Debugf(ctx, "ovn-kubernetes pod-subnet discovery: malformed node-subnets annotation %q", raw)
	return nil
}

// ovnConfigNetCIDRs reads the cluster-wide pod CIDRs from the ovn-config
// ConfigMap net_cidr key, trying the upstream then the OpenShift namespace.
func ovnConfigNetCIDRs(ctx context.Context, clients Clients) (cidrs []string, denied bool) {
	for _, ns := range []string{"ovn-kubernetes", "openshift-ovn-kubernetes"} {
		cm, err := clients.Typed.CoreV1().ConfigMaps(ns).Get(ctx, "ovn-config", metav1.GetOptions{})
		if err != nil {
			if deniedReadingResource(ctx, ovnDiscovererName, "the "+ns+"/ovn-config ConfigMap", err) {
				return nil, true
			}
			continue
		}
		raw := strings.TrimSpace(cm.Data["net_cidr"])
		if raw == "" {
			continue
		}
		return parseOVNNetCIDR(raw), false
	}
	return nil, false
}

// parseOVNNetCIDR parses the net_cidr value, one CIDR/hostPrefixLen entry per
// family, comma-separated. The host-subnet length after the final '/' is
// dropped (e.g. "10.128.0.0/14/23" -> "10.128.0.0/14").
func parseOVNNetCIDR(raw string) []string {
	var out []string
	for entry := range strings.SplitSeq(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Count(entry, "/") >= 2 {
			entry = entry[:strings.LastIndex(entry, "/")]
		}
		out = append(out, entry)
	}
	return out
}

// ovnOpenShiftClusterNetworkCIDRs reads status.clusterNetwork[].cidr from the
// OpenShift Network config (object "cluster"), gated on the
// networks.config.openshift.io CRD being installed.
func ovnOpenShiftClusterNetworkCIDRs(ctx context.Context, clients Clients) (cidrs []string, denied bool) {
	if clients.APIExtensions == nil || clients.Dynamic == nil {
		return nil, false
	}
	gvr, found, denied := firstServedCRD(ctx, clients, ovnDiscovererName, "networks.config.openshift.io")
	if denied {
		return nil, true
	}
	if !found {
		// Not OpenShift.
		return nil, false
	}
	obj, err := clients.Dynamic.Resource(gvr).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, deniedReadingResource(ctx, ovnDiscovererName, "the OpenShift Network config", err)
	}
	entries, found, _ := unstructured.NestedSlice(obj.Object, "status", "clusterNetwork")
	if !found {
		return nil, false
	}
	for _, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if cidr, ok := m["cidr"].(string); ok && cidr != "" {
			cidrs = append(cidrs, cidr)
		}
	}
	return cidrs, false
}
