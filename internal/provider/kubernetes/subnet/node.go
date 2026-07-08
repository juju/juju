// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const nodeDiscovererName = "node-pod-cidr"

// kubeRouterPodCIDRAnnotationKeys are the kube-router pod-CIDR override
// annotations: the current plural key (v2.0+, comma-separated, dual-stack) and
// the legacy singular key. kube-router never writes them itself, but an admin
// may. Both are read and unioned with node.Spec.PodCIDR(s).
var kubeRouterPodCIDRAnnotationKeys = []string{
	"kube-router.io/pod-cidrs",
	"kube-router.io/pod-cidr",
}

// nodePodCIDRDiscoverer is the final, generic catch-all link. A populated
// node.Spec.PodCIDR(s) is authoritative only for CNIs that allocate pod IPs
// from kube-controller-manager node-CIDR allocation; the CNI-specific
// discoverers run first and read the node spec themselves for their
// kcm-allocated modes, so by the time control reaches here the remaining real
// consumer is kube-router (which has no CRD/ConfigMap to key on).
type nodePodCIDRDiscoverer struct{}

func (nodePodCIDRDiscoverer) Name() string { return nodeDiscovererName }

// Discover returns node PodCIDRs and kube-router overrides as the final
// catch-all strategy for kube-controller-manager allocated pod networks.
func (nodePodCIDRDiscoverer) Discover(ctx context.Context, clients Clients) ([]string, error) {
	if clients.Typed == nil {
		return nil, nil
	}
	nodes, err := clients.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		if deniedReadingResource(ctx, nodeDiscovererName, "Nodes", err) {
			return fallbackCIDRs, nil
		}
		return nil, nil
	}
	var out []string
	for _, node := range nodes.Items {
		out = append(out, node.Spec.PodCIDRs...)
		if node.Spec.PodCIDR != "" {
			out = append(out, node.Spec.PodCIDR)
		}
		for _, key := range kubeRouterPodCIDRAnnotationKeys {
			raw := strings.TrimSpace(node.Annotations[key])
			if raw == "" {
				continue
			}
			out = append(out, splitCIDRCandidates(raw)...)
		}
	}
	return out, nil
}
