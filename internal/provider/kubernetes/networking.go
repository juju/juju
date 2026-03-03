// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"net"
	"sort"
	"strings"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

var _ environs.Networking = (*environNetworking)(nil)

type environNetworking struct {
	environs.NoContainerAddressesEnviron
	environs.NoSpaceDiscoveryEnviron
	listNodes func(context.Context) ([]corev1.Node, error)
}

var nodeCIDRAnnotationKeys = []string{
	"projectcalico.org/IPv4Address",
	"projectcalico.org/IPv6Address",
	"cilium.io/ipv4-pod-cidr",
	"cilium.io/ipv6-pod-cidr",
	"kube-router.io/pod-cidr",
}

// Subnets is part of the [environs.Networking] interface.
func (en environNetworking) Subnets(ctx context.Context, _ []network.Id) ([]network.SubnetInfo, error) {
	if en.listNodes == nil {
		return network.FallbackSubnetInfo, nil
	}

	nodes, err := en.listNodes(ctx)
	if err != nil {
		logger.Warningf(ctx, "unable to list kubernetes nodes for subnet discovery, using fallback subnets: %v", err)
		return network.FallbackSubnetInfo, nil
	}

	subnets := subnetsFromNodePodCIDRs(ctx, nodes)
	if len(subnets) == 0 {
		return network.FallbackSubnetInfo, nil
	}
	return subnets, nil
}

func subnetsFromNodePodCIDRs(ctx context.Context, nodes []corev1.Node) []network.SubnetInfo {
	seen := make(map[string]struct{})
	result := make([]network.SubnetInfo, 0)
	for _, node := range nodes {
		for _, cidrCandidate := range nodeCIDRCandidates(node) {
			_, ipNet, err := net.ParseCIDR(cidrCandidate)
			if err != nil {
				logger.Warningf(ctx, "ignoring invalid CIDR %q for node %q", cidrCandidate, node.Name)
				continue
			}
			cidr := ipNet.String()
			if _, exists := seen[cidr]; exists {
				continue
			}
			seen[cidr] = struct{}{}
			result = append(result, network.SubnetInfo{
				CIDR:       cidr,
				ProviderId: network.Id(cidr),
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CIDR < result[j].CIDR
	})
	return result
}

func nodeCIDRCandidates(node corev1.Node) []string {
	out := make([]string, 0, len(node.Spec.PodCIDRs)+1+len(nodeCIDRAnnotationKeys))
	out = append(out, node.Spec.PodCIDRs...)
	if node.Spec.PodCIDR != "" {
		out = append(out, node.Spec.PodCIDR)
	}
	if len(node.Annotations) == 0 {
		return out
	}
	for _, key := range nodeCIDRAnnotationKeys {
		raw := strings.TrimSpace(node.Annotations[key])
		if raw == "" {
			continue
		}
		out = append(out, splitCIDRCandidates(raw)...)
	}
	return out
}

func splitCIDRCandidates(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})
}

func (k *kubernetesClient) listNodes(ctx context.Context) ([]corev1.Node, error) {
	nodes, err := k.client().CoreV1().Nodes().List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, errors.Annotate(err, "listing kubernetes nodes")
	}
	return nodes.Items, nil
}

// NetworkInterfaces is part of the [environs.Networking] interface.
func (environNetworking) NetworkInterfaces(ctx context.Context, ids []instance.Id) ([]network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("network interfaces")
}

// SupportsSpaces is part of the [environs.Networking] interface.
func (environNetworking) SupportsSpaces() (bool, error) {
	return false, nil
}
