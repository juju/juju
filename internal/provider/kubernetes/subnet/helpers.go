// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"context"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"

	k8s "github.com/juju/juju/caas/kubernetes"
)

// deniedReadingResource classifies an error from a pod-CIDR read. It reports
// whether the error was an RBAC denial (Forbidden): when true, the caller must
// return the fallback CIDRs to short-circuit the chain, and a warning with
// remediation guidance has been logged. Absent / no-match / transient errors
// are logged at debug and false is returned, so the caller contributes nothing.
func deniedReadingResource(ctx context.Context, discoverer, resource string, err error) bool {
	if k8serrors.IsForbidden(err) {
		logger.Warningf(ctx,
			"%s pod-subnet discovery: permission denied reading %s; grant the "+
				"model's Kubernetes credential read access to %s to enable accurate "+
				"pod-subnet discovery. Using the default %v fallback subnets: %v",
			discoverer, resource, resource, fallbackCIDRs, err)
		return true
	}
	switch {
	case k8serrors.IsNotFound(err),
		meta.IsNoMatchError(err),
		discovery.IsGroupDiscoveryFailedError(err):
		logger.Debugf(ctx, "%s pod-subnet discovery: %s not present; skipping: %v",
			discoverer, resource, err)
	default:
		logger.Debugf(ctx, "%s pod-subnet discovery: ignoring error reading %s: %v",
			discoverer, resource, err)
	}
	return false
}

// firstServedCRD resolves the first of the named CRDs (each "<plural>.<group>")
// that is installed and has a served version, returning a GVR built from that
// served version. found is false when none are installed/served. denied is true
// when reading the CRD definitions was Forbidden, in which case the caller must
// short-circuit to the fallback.
func firstServedCRD(
	ctx context.Context, clients Clients, discoverer string, names ...string,
) (gvr schema.GroupVersionResource, found bool, denied bool) {
	if clients.APIExtensions == nil {
		return schema.GroupVersionResource{}, false, false
	}
	for _, name := range names {
		crd, err := clients.APIExtensions.ApiextensionsV1().
			CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if deniedReadingResource(ctx, discoverer, "the "+name+" CRD definition", err) {
				return schema.GroupVersionResource{}, false, true
			}
			continue
		}
		for _, v := range crd.Spec.Versions {
			if !v.Served {
				continue
			}
			return schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  v.Name,
				Resource: crd.Spec.Names.Plural,
			}, true, false
		}
		logger.Debugf(ctx, "%s pod-subnet discovery: CRD %q installed but has no served version",
			discoverer, name)
	}
	return schema.GroupVersionResource{}, false, false
}

// nodeSpecCIDRs returns the union of node.Spec.PodCIDR(s) across all nodes.
// denied reports an RBAC denial reading Nodes.
func nodeSpecCIDRs(ctx context.Context, clients Clients, discoverer string) (cidrs []string, denied bool) {
	nodes, err := clients.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, deniedReadingResource(ctx, discoverer, "Nodes", err)
	}
	for _, node := range nodes.Items {
		cidrs = append(cidrs, node.Spec.PodCIDRs...)
		if node.Spec.PodCIDR != "" {
			cidrs = append(cidrs, node.Spec.PodCIDR)
		}
	}
	return cidrs, false
}

// logFallbackWarning emits a warning when discovery found nothing. On a managed
// VPC-native cluster (EKS/AKS/GKE, excluding MicroK8s) it explains why the
// fallback was necessary; otherwise it logs a generic notice.
func logFallbackWarning(ctx context.Context, clients Clients) {
	switch detectManagedCloud(ctx, clients) {
	case k8s.K8sCloudEC2, k8s.K8sCloudAzure, k8s.K8sCloudGCE:
		logger.Warningf(ctx,
			"no pod subnets discovered: this looks like a managed cloud cluster "+
				"(EKS/AKS/GKE) using VPC-native networking, where pods receive "+
				"routable cloud IPs and there is no pod CIDR to discover. Install a "+
				"supported overlay CNI (Calico, Cilium, kube-router, OVN-Kubernetes "+
				"or Kube-OVN) if subnet discovery is required. Using the default %v "+
				"fallback subnets.", fallbackCIDRs)
	default:
		logger.Warningf(ctx,
			"no pod subnets discovered; no supported CNI source was found. "+
				"Using the default %v fallback subnets.",
			fallbackCIDRs)
	}
}

// detectManagedCloud returns the managed cloud type (EC2/Azure/GCE) when any
// node matches its checkers, or "" otherwise. MicroK8s is excluded as it ships
// its own CNI.
func detectManagedCloud(ctx context.Context, clients Clients) string {
	if clients.Typed == nil || clients.CloudDetector == nil {
		return ""
	}
	nodes, err := clients.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Debugf(ctx, "unable to list nodes for cloud detection: %v", err)
		return ""
	}
	for _, node := range nodes.Items {
		cloudType, _ := clients.CloudDetector(node)
		switch cloudType {
		case k8s.K8sCloudEC2, k8s.K8sCloudAzure, k8s.K8sCloudGCE:
			return cloudType
		}
	}
	return ""
}
