// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	"github.com/juju/errors"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/kubernetes/subnet"
)

var _ environs.Networking = (*environNetworking)(nil)

type environNetworking struct {
	environs.NoContainerAddressesEnviron
	environs.NoSpaceDiscoveryEnviron

	clientset     kubernetes.Interface
	dynamicClient dynamic.Interface
	apiExtClient  apiextensionsclientset.Interface
}

func newEnvironNetworking(
	k8sClient kubernetes.Interface,
	apiExtClient apiextensionsclientset.Interface,
	dynamicClient dynamic.Interface,
) environNetworking {
	return environNetworking{
		clientset:     k8sClient,
		dynamicClient: dynamicClient,
		apiExtClient:  apiExtClient,
	}
}

func (en environNetworking) clients() subnet.Clients {
	return subnet.Clients{
		Typed:         en.clientset,
		Dynamic:       en.dynamicClient,
		APIExtensions: en.apiExtClient,
		CloudDetector: GetCloudRegionFromNodeMeta,
	}
}

// Subnets is part of the [environs.Networking] interface.
func (en environNetworking) Subnets(ctx context.Context, _ []network.Id) ([]network.SubnetInfo, error) {
	if en.clientset == nil {
		return network.FallbackSubnetInfo, nil
	}
	return subnet.Subnets(ctx, en.clients())
}

// NetworkInterfaces is part of the [environs.Networking] interface.
func (environNetworking) NetworkInterfaces(ctx context.Context, ids []instance.Id) ([]network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("network interfaces")
}

// SupportsSpaces is part of the [environs.Networking] interface.
func (environNetworking) SupportsSpaces() (bool, error) {
	return false, nil
}
