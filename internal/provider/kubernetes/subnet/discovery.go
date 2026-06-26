// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/core/network"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.kubernetes.provider.subnetsdiscovery")

// CloudDetector inspects a node's labels and returns the cloud provider type
// and region, or empty strings when no recognised cloud is found.
type CloudDetector func(node corev1.Node) (cloudType string, region string)

// Clients bundles the Kubernetes clients each discoverer may use: the typed
// clientset (core resources: Nodes, ConfigMaps), the dynamic client (CNI custom
// resource objects, read as unstructured with an explicit GVR), and the
// apiextensions client (CRD definitions — installation and served-version
// detection). CloudDetector provides cloud-type awareness for fallback
// diagnostics without importing the parent kubernetes package.
type Clients struct {
	Typed         kubernetes.Interface
	Dynamic       dynamic.Interface
	APIExtensions apiextensionsclientset.Interface
	CloudDetector CloudDetector
}

// Subnets is the primary entry point for pod-subnet discovery. It runs the full
// discoverer chain, normalises the result, and handles fallback logic. The
// chain ordering is an internal implementation detail.
func Subnets(ctx context.Context, clients Clients) ([]network.SubnetInfo, error) {
	cidrs := Resolve(ctx, clients, chain()...)
	subnets := normalizeSubnets(ctx, cidrs)
	if len(subnets) == 0 {
		logFallbackWarning(ctx, clients)
		return network.FallbackSubnetInfo, nil
	}
	return subnets, nil
}
