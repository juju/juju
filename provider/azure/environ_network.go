// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdcontext "context"

	azurenetwork "github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

var _ environs.NetworkingEnviron = &azureEnviron{}

// SupportsSpaces implements environs.NetworkingEnviron.
func (env *azureEnviron) SupportsSpaces(context.ProviderCallContext) (bool, error) {
	return false, nil
}

// Subnets implements environs.NetworkingEnviron.
func (env *azureEnviron) Subnets(
	_ context.ProviderCallContext, instanceID instance.Id, _ []network.Id) ([]network.SubnetInfo, error) {
	if instanceID != instance.UnknownId {
		return nil, errors.NotSupportedf("subnets for instance")
	}

	netName := env.config.virtualNetworkName
	if netName == "" {
		netName = internalNetworkName
	}

	// Subnet discovery happens immediately after model creation.
	// We need to ensure that the asynchronously invoked resource creation has
	// completed and added our networking assets.
	if err := env.waitCommonResourcesCreated(); err != nil {
		return nil, errors.Annotate(
			err, "waiting for common resources to be created",
		)
	}

	subClient := azurenetwork.SubnetsClient{BaseClient: env.network}
	subnets, err := subClient.List(stdcontext.Background(), env.resourceGroup, netName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	values := subnets.Values()
	results := make([]network.SubnetInfo, len(values))
	for i, sub := range values {
		id := to.String(sub.ID)

		// An empty CIDR is no use to us, so guard against it.
		cidr := to.String(sub.AddressPrefix)
		if cidr == "" {
			logger.Debugf("ignoring subnet %q with empty address prefix", id)
			continue
		}

		results[i] = network.SubnetInfo{
			CIDR:       cidr,
			ProviderId: network.Id(id),
		}
	}
	return results, nil
}

// SuperSubnets implements environs.NetworkingEnviron.
func (env *azureEnviron) SuperSubnets(context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

// SupportsSpaceDiscovery implements environs.NetworkingEnviron.
func (env *azureEnviron) SupportsSpaceDiscovery(context.ProviderCallContext) (bool, error) {
	return false, nil
}

// Spaces implements environs.NetworkingEnviron.
func (env *azureEnviron) Spaces(context.ProviderCallContext) ([]network.SpaceInfo, error) {
	return nil, errors.NotSupportedf("spaces")
}

// SupportsContainerAddresses implements environs.NetworkingEnviron.
func (env *azureEnviron) SupportsContainerAddresses(context.ProviderCallContext) (bool, error) {
	return false, nil
}

// AllocateContainerAddresses implements environs.NetworkingEnviron.
func (env *azureEnviron) AllocateContainerAddresses(
	context.ProviderCallContext, instance.Id, names.MachineTag, network.InterfaceInfos,
) (network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("container addresses")
}

// ReleaseContainerAddresses implements environs.NetworkingEnviron.
func (env *azureEnviron) ReleaseContainerAddresses(context.ProviderCallContext, []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container addresses")
}

// ProviderSpaceInfo implements environs.NetworkingEnviron.
func (*azureEnviron) ProviderSpaceInfo(
	context.ProviderCallContext, *network.SpaceInfo,
) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// AreSpacesRoutable implements environs.NetworkingEnviron.
func (*azureEnviron) AreSpacesRoutable(_ context.ProviderCallContext, _, _ *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// SSHAddresses implements environs.NetworkingEnviron.
func (*azureEnviron) SSHAddresses(
	_ context.ProviderCallContext, addresses network.SpaceAddresses) (network.SpaceAddresses, error) {
	return addresses, nil
}

// NetworkInterfaces implements environs.NetworkingEnviron.
func (env *azureEnviron) NetworkInterfaces(
	context.ProviderCallContext, []instance.Id,
) ([]network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("network interfaces")
}
