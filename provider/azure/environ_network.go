// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdcontext "context"
	"fmt"
	"math/rand"
	"strings"

	azurenetwork "github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

var _ environs.NetworkingEnviron = &azureEnviron{}

// SupportsSpaces implements environs.NetworkingEnviron.
func (env *azureEnviron) SupportsSpaces(context.ProviderCallContext) (bool, error) {
	return true, nil
}

func (env *azureEnviron) networkInfo() (vnetRG string, vnetName string) {
	// The virtual network to use defaults to "juju-internal-network"
	// but may also be specified by the user.
	vnetName = internalNetworkName
	vnetRG = env.resourceGroup
	if env.config.virtualNetworkName != "" {
		// network may be "mynetwork" or "resourceGroup/mynetwork"
		parts := strings.Split(env.config.virtualNetworkName, "/")
		vnetName = parts[0]
		if len(parts) > 1 {
			vnetRG = parts[0]
			vnetName = parts[1]
		}
		logger.Debugf("user specified network name %q in resource group %q", vnetName, vnetRG)
	}
	return
}

// Subnets implements environs.NetworkingEnviron.
func (env *azureEnviron) Subnets(
	_ context.ProviderCallContext, instanceID instance.Id, _ []network.Id) ([]network.SubnetInfo, error) {
	if instanceID != instance.UnknownId {
		return nil, errors.NotSupportedf("subnets for instance")
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
	vnetRG, vnetName := env.networkInfo()
	subnets, err := subClient.List(stdcontext.Background(), vnetRG, vnetName)
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

// networkInfoForInstance returns the virtual network and subnet to use
// when provisioning an instance.
func (env *azureEnviron) networkInfoForInstance(
	instanceConfig *instancecfg.InstanceConfig,
	constraints constraints.Value,
	availabilityZone string,
	subnetsToZones []map[network.Id][]string,
) (vnetID string, subnetIDs []string, _ error) {

	// Controller and non-controller machines are assigned to separate
	// subnets. This enables us to create controller-specific NSG rules
	// just by targeting the controller subnet.
	subnetName := internalSubnetName
	if instanceConfig.Controller != nil {
		subnetName = controllerSubnetName
	}
	// The subnet belongs to a virtual network.
	vnetRG, vnetName := env.networkInfo()
	vnetID = fmt.Sprintf(`[resourceId('Microsoft.Network/virtualNetworks', '%s')]`, vnetName)
	subnetID := fmt.Sprintf(
		`[concat(resourceId('Microsoft.Network/virtualNetworks', '%s'), '/subnets/%s')]`,
		vnetName, subnetName,
	)
	if vnetRG != "" {
		vnetID = fmt.Sprintf(`[resourceId('%s', 'Microsoft.Network/virtualNetworks', '%s')]`, vnetRG, vnetName)
		subnetID = fmt.Sprintf(
			`[concat(resourceId('%s', 'Microsoft.Network/virtualNetworks', '%s'), '/subnets/%s')]`,
			vnetRG, vnetName, subnetName,
		)
	}

	// No space constraints so use the default network and subnet.
	if !constraints.HasSpaces() {
		return vnetID, []string{subnetID}, nil
	}

	// Attempt to filter the subnet IDs for the configured availability zone.
	// If there is no configured zone, consider all subnet IDs.
	az := availabilityZone
	subnetIDsForZone := make([][]network.Id, len(subnetsToZones))
	for i, nic := range subnetsToZones {
		var subnetIDs []network.Id
		if az != "" {
			var err error
			if subnetIDs, err = network.FindSubnetIDsForAvailabilityZone(az, nic); err != nil {
				return "", nil, errors.Annotatef(err, "getting subnets in zone %q", az)
			}
			if len(subnetIDs) == 0 {
				return "", nil, errors.Errorf("availability zone %q has no subnets satisfying space constraints", az)
			}
		} else {
			for subnetID := range nic {
				subnetIDs = append(subnetIDs, subnetID)
			}
		}

		// Filter out any fan networks.
		subnetIDsForZone[i] = network.FilterInFanNetwork(subnetIDs)
	}
	logger.Debugf("found subnet ids for zone: %#v", subnetIDsForZone)

	/// For each list of subnet IDs that satisfy space and zone constraints,
	// choose a single one at random.
	subnetIDForZone := make([]network.Id, len(subnetIDsForZone))
	for i, subnetIDs := range subnetIDsForZone {
		if len(subnetIDs) == 1 {
			subnetIDForZone[i] = subnetIDs[0]
			continue
		}

		subnetIDForZone[i] = subnetIDs[rand.Intn(len(subnetIDs))]
	}
	if len(subnetIDForZone) == 0 {
		return "", nil, errors.NotFoundf("subnet for constraint %q", constraints.String())
	}

	subnetIDs = make([]string, len(subnetIDForZone))
	for i, id := range subnetIDForZone {
		subnetIDs[i] = string(id)
	}
	return vnetID, subnetIDs, nil
}
