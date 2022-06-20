// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"math/rand"
	"strings"

	azurenetwork "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/azure/internal/errorutils"
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
	ctx context.ProviderCallContext, instanceID instance.Id, _ []network.Id) ([]network.SubnetInfo, error) {
	if instanceID != instance.UnknownId {
		return nil, errors.NotSupportedf("subnets for instance")
	}
	subnets, err := env.allSubnets(ctx)
	return subnets, errorutils.HandleCredentialError(err, ctx)
}

func (env *azureEnviron) allProviderSubnets(ctx context.ProviderCallContext) ([]*azurenetwork.Subnet, error) {
	// Subnet discovery happens immediately after model creation.
	// We need to ensure that the asynchronously invoked resource creation has
	// completed and added our networking assets.
	if err := env.waitCommonResourcesCreated(ctx); err != nil {
		return nil, errors.Annotate(
			err, "waiting for common resources to be created",
		)
	}

	vnetRG, vnetName := env.networkInfo()
	var result []*azurenetwork.Subnet
	pager := env.subnets.NewListPager(vnetRG, vnetName, nil)
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result = append(result, next.Value...)
	}
	return result, nil
}

func (env *azureEnviron) allSubnets(ctx context.ProviderCallContext) ([]network.SubnetInfo, error) {
	values, err := env.allProviderSubnets(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []network.SubnetInfo
	for _, sub := range values {
		id := toValue(sub.ID)
		if sub.Properties == nil {
			continue
		}
		// An empty CIDR is no use to us, so guard against it.
		cidr := toValue(sub.Properties.AddressPrefix)
		if cidr == "" {
			logger.Debugf("ignoring subnet %q with empty address prefix", id)
			continue
		}

		results = append(results, network.SubnetInfo{
			CIDR:       cidr,
			ProviderId: network.Id(id),
		})
	}
	return results, nil
}

func (env *azureEnviron) allPublicIPs(ctx context.ProviderCallContext) (map[string]network.ProviderAddress, error) {
	idToIPMap := make(map[string]network.ProviderAddress)

	pager := env.publicAddresses.NewListPager(env.resourceGroup, nil)
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, ipRes := range next.Value {
			if ipRes.ID == nil || ipRes.Properties == nil || ipRes.Properties.IPAddress == nil {
				continue
			}

			var cfgMethod = network.ConfigDHCP
			if toValue(ipRes.Properties.PublicIPAllocationMethod) == azurenetwork.IPAllocationMethodStatic {
				cfgMethod = network.ConfigStatic
			}

			idToIPMap[*ipRes.ID] = network.NewMachineAddress(
				toValue(ipRes.Properties.IPAddress),
				network.WithConfigType(cfgMethod),
			).AsProviderAddress()
		}
	}

	return idToIPMap, nil
}

// SuperSubnets implements environs.NetworkingEnviron.
func (env *azureEnviron) SuperSubnets(context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
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

// AreSpacesRoutable implements environs.NetworkingEnviron.
func (*azureEnviron) AreSpacesRoutable(_ context.ProviderCallContext, _, _ *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// SSHAddresses implements environs.NetworkingEnviron.
func (*azureEnviron) SSHAddresses(
	_ context.ProviderCallContext, addresses network.SpaceAddresses) (network.SpaceAddresses, error) {
	return addresses, nil
}

// NetworkInterfaces implements environs.NetworkingEnviron. It returns back
// a slice where the i_th element contains the list of network interfaces
// for the i_th provided instance ID.
//
// If none of the provided instance IDs exist, ErrNoInstances will be returned.
// If only a subset of the instance IDs exist, the result will contain a nil
// value for the missing instances and a ErrPartialInstances error will be
// returned.
func (env *azureEnviron) NetworkInterfaces(ctx context.ProviderCallContext, instanceIDs []instance.Id) ([]network.InterfaceInfos, error) {
	// Create a subnet (provider) ID to CIDR map so we can identify the
	// subnet for each NIC address when mapping azure NIC details.
	allSubnets, err := env.allSubnets(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetIDToCIDR := make(map[string]string)
	for _, sub := range allSubnets {
		subnetIDToCIDR[sub.ProviderId.String()] = sub.CIDR
	}

	instIfaceMap, err := instanceNetworkInterfaces(ctx, env.resourceGroup, env.interfaces)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Create a map of azure IP address IDs to provider addresses. We will
	// use this information to associate public IP addresses with NICs
	// when mapping the obtained azure NIC list.
	ipMap, err := env.allPublicIPs(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		res        = make([]network.InterfaceInfos, len(instanceIDs))
		matchCount int
	)

	for resIdx, instID := range instanceIDs {
		azInterfaceList, found := instIfaceMap[instID]
		if !found {
			continue
		}

		matchCount++
		res[resIdx] = mapAzureInterfaceList(azInterfaceList, subnetIDToCIDR, ipMap)
	}

	if matchCount == 0 {
		return nil, environs.ErrNoInstances
	} else if matchCount < len(instanceIDs) {
		return res, environs.ErrPartialInstances
	}

	return res, nil
}

func mapAzureInterfaceList(in []*azurenetwork.Interface, subnetIDToCIDR map[string]string, ipMap map[string]network.ProviderAddress) network.InterfaceInfos {
	var out = make(network.InterfaceInfos, len(in))

	for idx, azif := range in {
		ni := network.InterfaceInfo{
			DeviceIndex:   idx,
			Disabled:      false,
			NoAutoStart:   false,
			InterfaceType: network.EthernetDevice,
			Origin:        network.OriginProvider,
		}

		if azif.Properties != nil && azif.Properties.MacAddress != nil {
			ni.MACAddress = network.NormalizeMACAddress(toValue(azif.Properties.MacAddress))
		}
		if azif.ID != nil {
			ni.ProviderId = network.Id(*azif.ID)
		}

		if azif.Properties == nil || azif.Properties.IPConfigurations == nil {
			out[idx] = ni
			continue
		}

		for _, ipConf := range azif.Properties.IPConfigurations {
			if ipConf.Properties == nil {
				continue
			}

			isPrimary := ipConf.Properties.Primary != nil && toValue(ipConf.Properties.Primary)

			// Azure does not include the public IP address values
			// but it does provide us with the ID of any assigned
			// public addresses which we can use to index the ipMap.
			if ipConf.Properties.PublicIPAddress != nil && ipConf.Properties.PublicIPAddress.ID != nil {
				if providerAddr, found := ipMap[toValue(ipConf.Properties.PublicIPAddress.ID)]; found {
					// If this a primary address make sure it appears
					// at the top of the shadow address list.
					if isPrimary {
						ni.ShadowAddresses = append(network.ProviderAddresses{providerAddr}, ni.ShadowAddresses...)
						ni.ConfigType = providerAddr.AddressConfigType()
					} else {
						ni.ShadowAddresses = append(ni.ShadowAddresses, providerAddr)
					}
				}
			}

			// Check if the configuration also includes a private address component.
			if ipConf.Properties.PrivateIPAddress == nil {
				continue
			}

			var cfgMethod = network.ConfigDHCP
			if toValue(ipConf.Properties.PrivateIPAllocationMethod) == azurenetwork.IPAllocationMethodStatic {
				cfgMethod = network.ConfigStatic
			}

			addrOpts := []func(network.AddressMutator){
				network.WithScope(network.ScopeCloudLocal),
				network.WithConfigType(cfgMethod),
			}

			var subnetID string
			if ipConf.Properties.Subnet != nil && ipConf.Properties.Subnet.ID != nil {
				subnetID = toValue(ipConf.Properties.Subnet.ID)
				if subnetCIDR := subnetIDToCIDR[subnetID]; subnetCIDR != "" {
					addrOpts = append(addrOpts, network.WithCIDR(subnetCIDR))
				}
			}

			providerAddr := network.NewMachineAddress(
				toValue(ipConf.Properties.PrivateIPAddress),
				addrOpts...,
			).AsProviderAddress()

			// If this is the primary address ensure that it appears
			// at the top of the address list.
			if isPrimary {
				ni.Addresses = append(network.ProviderAddresses{providerAddr}, ni.Addresses...)
			} else {
				ni.Addresses = append(ni.Addresses, providerAddr)
			}

			// Record the subnetID and config mode to the NIC instance
			if isPrimary && subnetID != "" {
				ni.ProviderSubnetId = network.Id(subnetID)
				ni.ConfigType = cfgMethod
			}
		}

		out[idx] = ni
	}

	return out
}

// defaultControllerSubnet returns the subnet to use for starting a controller
// if not otherwise specified using a placement directive.
func (env *azureEnviron) defaultControllerSubnet() network.Id {
	// By default, controller and non-controller machines are assigned to separate
	// subnets. This enables us to create controller-specific NSG rules
	// just by targeting the controller subnet.

	vnetRG, vnetName := env.networkInfo()
	subnetID := fmt.Sprintf(
		`[concat(resourceId('Microsoft.Network/virtualNetworks', '%s'), '/subnets/%s')]`,
		vnetName, controllerSubnetName,
	)
	if vnetRG != "" {
		subnetID = fmt.Sprintf(
			`[concat(resourceId('%s', 'Microsoft.Network/virtualNetworks', '%s'), '/subnets/%s')]`,
			vnetRG, vnetName, controllerSubnetName,
		)
	}
	return network.Id(subnetID)
}

func (env *azureEnviron) findSubnetID(ctx context.ProviderCallContext, subnetName string) (network.Id, error) {
	subnets, err := env.allProviderSubnets(ctx)
	if err != nil {
		return "", errorutils.HandleCredentialError(err, ctx)
	}
	for _, subnet := range subnets {
		if toValue(subnet.Name) == subnetName {
			return network.Id(toValue(subnet.ID)), nil
		}
	}
	return "", errors.NotFoundf("subnet %q", subnetName)
}

// networkInfoForInstance returns the virtual network and subnet to use
// when provisioning an instance.
func (env *azureEnviron) networkInfoForInstance(
	ctx context.ProviderCallContext,
	args environs.StartInstanceParams,
	bootstrapping, controller bool,
	placementSubnetID network.Id,
) (vnetID string, subnetIDs []network.Id, _ error) {

	vnetRG, vnetName := env.networkInfo()
	vnetID = fmt.Sprintf(`[resourceId('Microsoft.Network/virtualNetworks', '%s')]`, vnetName)
	if vnetRG != "" {
		vnetID = fmt.Sprintf(`[resourceId('%s', 'Microsoft.Network/virtualNetworks', '%s')]`, vnetRG, vnetName)
	}

	constraints := args.Constraints

	// We'll collect all the possible subnets and pick one
	// based on constraints and placement.
	var possibleSubnets [][]network.Id

	if !constraints.HasSpaces() {
		// Use placement if specified.
		if placementSubnetID != "" {
			return vnetID, []network.Id{placementSubnetID}, nil
		}

		// When bootstrapping the network doesn't exist yet so just
		// return the relevant subnet ID and it is created as part of
		// the bootstrap process.
		if bootstrapping && env.config.virtualNetworkName == "" {
			return vnetID, []network.Id{env.defaultControllerSubnet()}, nil
		}

		// Prefer the legacy default subnet if found.
		defaultSubnetName := internalSubnetName
		if controller {
			defaultSubnetName = controllerSubnetName
		}
		defaultSubnetID, err := env.findSubnetID(ctx, defaultSubnetName)
		if err != nil && !errors.IsNotFound(err) {
			return "", nil, errors.Trace(err)
		}
		if err == nil {
			return vnetID, []network.Id{defaultSubnetID}, nil
		}

		// For deployments without a spaces constraint, there's no subnets to zones mapping.
		// So get all accessible subnets.
		allSubnets, err := env.allSubnets(ctx)
		if err != nil {
			return "", nil, errorutils.HandleCredentialError(errors.Trace(err), ctx)
		}
		subnetIds := make([]network.Id, len(allSubnets))
		for i, subnet := range allSubnets {
			subnetIds[i] = subnet.ProviderId
		}
		possibleSubnets = [][]network.Id{subnetIds}
	} else {
		var err error
		// Attempt to filter the subnet IDs for the configured availability zone.
		// If there is no configured zone, consider all subnet IDs.
		possibleSubnets, err = env.subnetsForZone(args.SubnetsToZones, args.AvailabilityZone)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
	}

	// For each list of subnet IDs that satisfy space and zone constraints,
	// choose a single one at random.
	var subnetIDForZone []network.Id
	for _, zoneSubnetIDs := range possibleSubnets {
		// Use placement to select a single subnet if needed.
		var subnetIDs []network.Id
		for _, id := range zoneSubnetIDs {
			if placementSubnetID == "" || placementSubnetID == id {
				subnetIDs = append(subnetIDs, id)
			}
		}
		if len(subnetIDs) == 1 {
			subnetIDForZone = append(subnetIDForZone, subnetIDs[0])
			continue
		} else if len(subnetIDs) > 0 {
			subnetIDForZone = append(subnetIDForZone, subnetIDs[rand.Intn(len(subnetIDs))])
		}
	}
	if len(subnetIDForZone) == 0 {
		return "", nil, errors.NotFoundf("subnet for constraint %q", constraints.String())
	}

	// Put any placement subnet first in the list
	// so it ia allocated to the primary NIC.
	if placementSubnetID != "" {
		subnetIDs = append(subnetIDs, placementSubnetID)
	}
	for _, id := range subnetIDForZone {
		if id != placementSubnetID {
			subnetIDs = append(subnetIDs, id)
		}
	}
	return vnetID, subnetIDs, nil
}

func (env *azureEnviron) subnetsForZone(subnetsToZones []map[network.Id][]string, az string) ([][]network.Id, error) {
	subnetIDsForZone := make([][]network.Id, len(subnetsToZones))
	for i, nic := range subnetsToZones {
		var subnetIDs []network.Id
		if az != "" {
			var err error
			if subnetIDs, err = network.FindSubnetIDsForAvailabilityZone(az, nic); err != nil {
				return nil, errors.Annotatef(err, "getting subnets in zone %q", az)
			}
			if len(subnetIDs) == 0 {
				return nil, errors.Errorf("availability zone %q has no subnets satisfying space constraints", az)
			}
		} else {
			for subnetID := range nic {
				subnetIDs = append(subnetIDs, subnetID)
			}
		}

		// Filter out any fan networks.
		subnetIDsForZone[i] = network.FilterInFanNetwork(subnetIDs)
	}
	return subnetIDsForZone, nil
}

func (env *azureEnviron) parsePlacement(placement string) (string, error) {
	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		return "", fmt.Errorf("unknown placement directive: %v", placement)
	}
	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "subnet":
		return value, nil
	}
	return "", fmt.Errorf("unknown placement directive: %v", placement)
}

func (env *azureEnviron) findPlacementSubnet(ctx context.ProviderCallContext, placement string) (network.Id, error) {
	if placement == "" {
		return "", nil
	}
	subnetName, err := env.parsePlacement(placement)
	if err != nil {
		return "", errors.Trace(err)
	}

	logger.Debugf("searching for subnet matching placement directive %q", subnetName)
	return env.findSubnetID(ctx, subnetName)
}
