// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	kubeOVNDiscovererName = "kube-ovn"

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

// Discover detects Kube-OVN from its Subnet CRD and returns pod CIDRs after
// excluding the join subnet and preferring the default VPC.
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
