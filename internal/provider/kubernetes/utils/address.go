// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"

	core "k8s.io/api/core/v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
)

// ControllerPodFQDN returns the stable, cluster-resolvable per-pod DNS name
// assigned to the controller pod named podName in the given namespace by the
// controller headless service. It is the single source of truth for the
// controller pod FQDN string. The form is:
//
//	<pod-name>.controller-service-endpoints.<namespace>.svc.cluster.local
//
// for example:
//
//	controller-0.controller-service-endpoints.controller-a.svc.cluster.local
func ControllerPodFQDN(podName, namespace string) string {
	return fmt.Sprintf(
		"%s.%s.%s.%s",
		podName,
		constants.ControllerServiceEndpointsName,
		namespace,
		constants.ClusterLocalDomain,
	)
}

// GetSvcAddresses returns the network addresses for the given service.
func GetSvcAddresses(svc *core.Service, includeClusterIP bool) []network.ProviderAddress {
	var netAddrs []network.ProviderAddress

	addressExist := func(addr string) bool {
		for _, v := range netAddrs {
			if addr == v.Value {
				return true
			}
		}
		return false
	}
	appendUniqueAddrs := func(scope network.Scope, addrs ...string) {
		for _, v := range addrs {
			if v != "" && v != "None" && !addressExist(v) {
				netAddrs = append(netAddrs, network.NewMachineAddress(v, network.WithScope(scope)).AsProviderAddress())
			}
		}
	}

	t := svc.Spec.Type
	clusterIP := svc.Spec.ClusterIP
	switch t {
	case core.ServiceTypeClusterIP:
		appendUniqueAddrs(network.ScopeCloudLocal, clusterIP)
	case core.ServiceTypeExternalName:
		appendUniqueAddrs(network.ScopePublic, svc.Spec.ExternalName)
	case core.ServiceTypeNodePort:
		appendUniqueAddrs(network.ScopePublic, svc.Spec.ExternalIPs...)
	case core.ServiceTypeLoadBalancer:
		appendUniqueAddrs(network.ScopePublic, getLoadBalancerAddresses(svc)...)
	}
	if includeClusterIP {
		// append clusterIP as a fixed internal address.
		appendUniqueAddrs(network.ScopeCloudLocal, clusterIP)
	}
	return netAddrs
}

func getLoadBalancerAddresses(svc *core.Service) []string {
	// different cloud providers have a different way to report back the Load Balancer address.
	// This covers the cases we know about so far.
	var addr []string
	lpAdd := svc.Spec.LoadBalancerIP
	if lpAdd != "" {
		addr = append(addr, lpAdd)
	}

	ing := svc.Status.LoadBalancer.Ingress
	if len(ing) == 0 {
		return addr
	}

	for _, ingressAddr := range ing {
		if ingressAddr.IP != "" {
			addr = append(addr, ingressAddr.IP)
		}
		if ingressAddr.Hostname != "" {
			addr = append(addr, ingressAddr.Hostname)
		}
	}
	return addr
}
