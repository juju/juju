// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// -----------------------------------------------------------------------------
// Calico — calicoDiscoverer
// -----------------------------------------------------------------------------

// calicoDiscoverer reads Calico's authoritative pod-CIDR source. Under the
// default calico-ipam the pool CIDRs come from IPPool CRDs; under host-local
// IPAM (Canal, GKE) the node spec is authoritative. The IPPool CRD's presence
// (resolved via the apiextensions client) and whether any qualifying pool
// exists decides which mode is in effect.
type calicoDiscoverer struct{}

func (calicoDiscoverer) Name() string { return calicoDiscovererName }

func (calicoDiscoverer) Discover(ctx context.Context, clients Clients) ([]string, error) {
	if clients.APIExtensions == nil || clients.Dynamic == nil {
		return nil, nil
	}
	// crd.projectcalico.org/v1 (standard manifests, Canal, RKE2) is tried first
	// for widest compatibility, then projectcalico.org/v3 (Tigera operator).
	gvr, found, denied := firstServedCRD(ctx, clients, calicoDiscovererName,
		"ippools.crd.projectcalico.org", "ippools.projectcalico.org")
	if denied {
		return fallbackCIDRs, nil
	}
	if !found {
		// IPPool CRD absent: Calico is not present.
		return nil, nil
	}

	pools, denied := calicoIPPoolCIDRs(ctx, clients, gvr)
	if denied {
		return fallbackCIDRs, nil
	}
	if len(pools) > 0 {
		// calico-ipam: pool CIDRs are authoritative; ignore node.Spec.PodCIDR.
		return pools, nil
	}

	// IPPool CRD installed but no qualifying pools: host-local / Canal, where
	// node.Spec.PodCIDR(s) is authoritative (optionally supplemented by the
	// canal-config cluster pod CIDR).
	cidrs, denied := nodeSpecCIDRs(ctx, clients, calicoDiscovererName)
	if denied {
		return fallbackCIDRs, nil
	}
	if cidr := canalNetworkCIDR(ctx, clients); cidr != "" {
		cidrs = append(cidrs, cidr)
	}
	return cidrs, nil
}

// calicoIPPoolCIDRs lists Calico IPPool objects on the served GVR and returns
// the CIDRs of the pools that are pod-IP sources.
func calicoIPPoolCIDRs(ctx context.Context, clients Clients, gvr schema.GroupVersionResource) (cidrs []string, denied bool) {
	list, err := clients.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, deniedReadingResource(ctx, calicoDiscovererName, "Calico IPPool resources", err)
	}
	for i := range list.Items {
		pool := list.Items[i]
		if !calicoPoolIsPodSource(&pool) {
			continue
		}
		if cidr, ok, _ := unstructured.NestedString(pool.Object, "spec", "cidr"); ok && cidr != "" {
			cidrs = append(cidrs, cidr)
		}
	}
	return cidrs, false
}

// calicoPoolIsPodSource reports whether an IPPool contributes to the pod IP
// space: enabled, and either allowing the Workload use or with allowedUses
// unset (which defaults to Workload+Tunnel). assignmentMode Manual pools are
// included; nodeSelector is not considered.
func calicoPoolIsPodSource(pool *unstructured.Unstructured) bool {
	if disabled, ok, _ := unstructured.NestedBool(pool.Object, "spec", "disabled"); ok && disabled {
		return false
	}
	uses, ok, _ := unstructured.NestedStringSlice(pool.Object, "spec", "allowedUses")
	if !ok || len(uses) == 0 {
		return true
	}
	return slices.Contains(uses, "Workload")
}

// canalNetworkCIDR reads the cluster pod CIDR from the kube-system/canal-config
// ConfigMap's net-conf.json Network key (default 10.244.0.0/16). It returns ""
// when canal-config is absent or unparsable.
func canalNetworkCIDR(ctx context.Context, clients Clients) string {
	cm, err := clients.Typed.CoreV1().ConfigMaps("kube-system").Get(ctx, "canal-config", metav1.GetOptions{})
	if err != nil {
		logger.Debugf(ctx, "calico pod-subnet discovery: canal-config not read: %v", err)
		return ""
	}
	raw := strings.TrimSpace(cm.Data["net-conf.json"])
	if raw == "" {
		return ""
	}
	var netConf struct {
		Network string `json:"Network"`
	}
	if err := json.Unmarshal([]byte(raw), &netConf); err != nil {
		logger.Debugf(ctx, "calico pod-subnet discovery: malformed canal-config net-conf.json: %v", err)
		return ""
	}
	return strings.TrimSpace(netConf.Network)
}

// -----------------------------------------------------------------------------
// Cilium — ciliumDiscoverer
// -----------------------------------------------------------------------------

// ciliumDiscoverer reads Cilium's pod CIDRs from the source dictated by the
// IPAM mode, detected from the kube-system/cilium-config ConfigMap ipam key.
type ciliumDiscoverer struct{}

func (ciliumDiscoverer) Name() string { return ciliumDiscovererName }

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

// -----------------------------------------------------------------------------
// OVN-Kubernetes — ovnKubernetesDiscoverer
// -----------------------------------------------------------------------------

const (
	ovnNodeSubnetsAnnotation = "k8s.ovn.org/node-subnets"
	ovnDefaultNetworkName    = "default"
)

// ovnKubernetesDiscoverer unions the three OVN-Kubernetes pod-CIDR sources (the
// per-node node-subnets annotation, the ovn-config net_cidr ConfigMap, and the
// OpenShift Network config CRD) and returns the combined set as its single
// confident result.
type ovnKubernetesDiscoverer struct{}

func (ovnKubernetesDiscoverer) Name() string { return ovnDiscovererName }

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

// -----------------------------------------------------------------------------
// Kube-OVN — kubeOVNDiscoverer
// -----------------------------------------------------------------------------

const (
	kubeOVNLogicalSwitchAnnotation = "ovn.kubernetes.io/logical_switch"
	kubeOVNCIDRAnnotation          = "ovn.kubernetes.io/cidr"
	kubeOVNDefaultJoinSwitch       = "join"
	kubeOVNDefaultVPC              = "ovn-cluster"
	kubeOVNProtocolDual            = "Dual"
)

// kubeOVNDiscoverer reads pod CIDRs from Kube-OVN Subnet CRDs, excluding the
// join (node↔pod transit) subnet and preferring the default VPC.
type kubeOVNDiscoverer struct{}

func (kubeOVNDiscoverer) Name() string { return kubeOVNDiscovererName }

func (kubeOVNDiscoverer) Discover(ctx context.Context, clients Clients) ([]string, error) {
	if clients.APIExtensions == nil || clients.Dynamic == nil {
		return nil, nil
	}
	gvr, found, denied := firstServedCRD(ctx, clients, kubeOVNDiscovererName, "subnets.kubeovn.io")
	if denied {
		return fallbackCIDRs, nil
	}
	if !found {
		// Kube-OVN not present.
		return nil, nil
	}
	list, err := clients.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		if deniedReadingResource(ctx, kubeOVNDiscovererName, "Kube-OVN Subnet resources", err) {
			return fallbackCIDRs, nil
		}
		return nil, nil
	}

	joinName, joinCIDR, ok := kubeOVNJoinIdentifiers(ctx, clients)
	if !ok {
		logger.Warningf(ctx,
			"kube-ovn pod-subnet discovery: unable to identify the join subnet; "+
				"using the default %v fallback subnets", fallbackCIDRs)
		return fallbackCIDRs, nil
	}

	var defaultVPC, otherVPC []string
	for i := range list.Items {
		subnet := list.Items[i]
		if subnet.GetName() == joinName {
			continue
		}
		cidrBlock, _, _ := unstructured.NestedString(subnet.Object, "spec", "cidrBlock")
		if cidrBlock == "" {
			continue
		}
		if joinCIDR != "" && cidrBlock == joinCIDR {
			continue
		}
		protocol, _, _ := unstructured.NestedString(subnet.Object, "spec", "protocol")
		var blocks []string
		if protocol == kubeOVNProtocolDual {
			blocks = splitCIDRCandidates(cidrBlock)
		} else {
			blocks = []string{cidrBlock}
		}
		vpc, _, _ := unstructured.NestedString(subnet.Object, "spec", "vpc")
		if vpc == "" || vpc == kubeOVNDefaultVPC {
			defaultVPC = append(defaultVPC, blocks...)
		} else {
			otherVPC = append(otherVPC, blocks...)
		}
	}
	if len(defaultVPC) > 0 {
		return defaultVPC, nil
	}
	// Custom-VPC subnets are isolated; only used when no default-VPC subnet
	// is found.
	return otherVPC, nil
}

// kubeOVNJoinIdentifiers returns the join subnet's name and CIDR, derived
// from node annotations to be robust against a customised --node-switch. The
// name defaults to "join" when node annotations were read but no custom value
// is available. ok is false when nodes cannot be inspected reliably.
func kubeOVNJoinIdentifiers(ctx context.Context, clients Clients) (name, cidr string, ok bool) {
	name = kubeOVNDefaultJoinSwitch
	if clients.Typed == nil {
		return name, "", false
	}
	nodes, err := clients.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Debugf(ctx, "kube-ovn pod-subnet discovery: unable to list nodes for join subnet detection: %v", err)
		return name, "", false
	}
	for _, node := range nodes.Items {
		if ls := strings.TrimSpace(node.Annotations[kubeOVNLogicalSwitchAnnotation]); ls != "" {
			name = ls
		}
		if c := strings.TrimSpace(node.Annotations[kubeOVNCIDRAnnotation]); c != "" {
			cidr = c
		}
		if cidr != "" {
			break
		}
	}
	return name, cidr, true
}
