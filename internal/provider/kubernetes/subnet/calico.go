// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const calicoDiscovererName = "calico"

// calicoDiscoverer reads Calico's authoritative pod-CIDR source. Under the
// default calico-ipam the pool CIDRs come from IPPool CRDs; under host-local
// IPAM (Canal, GKE) the node spec is authoritative. The IPPool CRD's presence
// (resolved via the apiextensions client) and whether any qualifying pool
// exists decides which mode is in effect.
type calicoDiscoverer struct{}

func (calicoDiscoverer) Name() string { return calicoDiscovererName }

// Discover detects Calico through its IPPool CRD and returns either qualifying
// IPPool CIDRs or node-spec CIDRs for host-local IPAM.
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
	if cidr, err := canalNetworkCIDR(ctx, clients); err != nil {
		logger.Debugf(ctx, "calico pod-subnet discovery: %v", err)
	} else if cidr != "" {
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
// ConfigMap's net-conf.json Network key (default 10.244.0.0/16).
func canalNetworkCIDR(ctx context.Context, clients Clients) (string, error) {
	cm, err := clients.Typed.CoreV1().ConfigMaps("kube-system").Get(ctx, "canal-config", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	raw := strings.TrimSpace(cm.Data["net-conf.json"])
	if raw == "" {
		return "", nil
	}
	var netConf struct {
		Network string `json:"Network"`
	}
	if err := json.Unmarshal([]byte(raw), &netConf); err != nil {
		return "", err
	}
	return strings.TrimSpace(netConf.Network), nil
}
