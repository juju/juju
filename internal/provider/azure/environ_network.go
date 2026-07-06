// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	azurenetwork "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/ipfamily"
	"github.com/juju/juju/environs"
)

var _ environs.NetworkingEnviron = (*azureEnviron)(nil)

// SupportsSpaces implements environs.NetworkingEnviron.
func (env *azureEnviron) SupportsSpaces() (bool, error) {
	return true, nil
}

func (env *azureEnviron) networkInfo(ctx context.Context) (vnetRG string, vnetName string) {
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
		logger.Debugf(ctx, "user specified network name %q in resource group %q", vnetName, vnetRG)
	}
	return
}

// Subnets implements environs.NetworkingEnviron.
func (env *azureEnviron) Subnets(
	ctx context.Context, _ []network.Id) ([]network.SubnetInfo, error) {
	subnets, err := env.allSubnets(ctx)
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}
	return subnets, nil
}

func (env *azureEnviron) allProviderSubnets(ctx context.Context) ([]*azurenetwork.Subnet, error) {
	// Subnet discovery happens immediately after model creation.
	// We need to ensure that the asynchronously invoked resource creation has
	// completed and added our networking assets.
	if err := env.waitCommonResourcesCreated(ctx); err != nil {
		return nil, errors.Annotate(
			err, "waiting for common resources to be created",
		)
	}

	subnets, err := env.subnetsClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	vnetRG, vnetName := env.networkInfo(ctx)
	var result []*azurenetwork.Subnet
	pager := subnets.NewListPager(vnetRG, vnetName, nil)
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result = append(result, next.Value...)
	}
	return result, nil
}

func (env *azureEnviron) allSubnets(ctx context.Context) ([]network.SubnetInfo, error) {
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

		// Prefer AddressPrefixes (plural) over AddressPrefix (singular).
		// Both may be set after PR #22736 (dual-stack VNet templates).
		// Single-prefix subnets have AddressPrefix set; multi-prefix (dual-stack)
		// have AddressPrefixes set, and AddressPrefix is nil.
		prefixes := subnetAddressPrefixes(sub.Properties)

		if len(prefixes) == 0 {
			logger.Debugf(ctx, "ignoring subnet %q with empty address prefix", id)
			continue
		}

		// Emit one SubnetInfo per prefix. Use the :ipv6 suffix to distinguish
		// IPv6 prefixes from IPv4, so that dual-stack subnets appear as two
		// distinct Juju subnets (one per family).
		for _, prefix := range prefixes {
			addrType, err := network.CIDRAddressType(prefix)
			if err != nil {
				logger.Debugf(ctx, "invalid CIDR %q in subnet %q: %v; skipping", prefix, id, err)
				continue
			}

			isIPv6 := addrType == network.IPv6Address
			results = append(results, network.SubnetInfo{
				CIDR:       prefix,
				ProviderId: subnetProviderIDForFamily(id, isIPv6),
			})
		}
	}
	return results, nil
}

func (env *azureEnviron) allPublicIPs(ctx context.Context) (map[string]network.ProviderAddress, error) {
	idToIPMap := make(map[string]network.ProviderAddress)

	pipClient, err := env.publicAddressesClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	pager := pipClient.NewListPager(env.resourceGroup, nil)
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

// NetworkInterfaces implements environs.NetworkingEnviron. It returns back
// a slice where the i_th element contains the list of network interfaces
// for the i_th provided instance ID.
//
// If none of the provided instance IDs exist, ErrNoInstances will be returned.
// If only a subset of the instance IDs exist, the result will contain a nil
// value for the missing instances and a ErrPartialInstances error will be
// returned.
func (env *azureEnviron) NetworkInterfaces(ctx context.Context, instanceIDs []instance.Id) ([]network.InterfaceInfos, error) {
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

	instIfaceMap, err := env.instanceNetworkInterfaces(ctx, env.resourceGroup)
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

			machineAddrOpts := []func(network.AddressMutator){
				network.WithScope(network.ScopeCloudLocal),
				network.WithConfigType(cfgMethod),
			}

			var (
				subnetID         string
				providerAddrOpts []func(address network.ProviderAddressMutator)
			)
			if ipConf.Properties.Subnet != nil && ipConf.Properties.Subnet.ID != nil {
				subnetID = toValue(ipConf.Properties.Subnet.ID)

				// Determine if this is an IPv6 IP configuration to add the
				// correct family-suffixed ProviderId and lookup the matching CIDR.
				isIPv6 := ipConf.Properties.PrivateIPAddressVersion != nil &&
					toValue(ipConf.Properties.PrivateIPAddressVersion) == azurenetwork.IPVersionIPv6

				// Build the Juju ProviderId suffix convention: bare for IPv4, :ipv6 for IPv6.
				jujuSubnetID := subnetProviderIDForFamily(subnetID, isIPv6)
				providerAddrOpts = append(providerAddrOpts, network.WithProviderSubnetID(jujuSubnetID))

				// Look up the CIDR for this family-specific subnet. If we have a dual-stack
				// subnet, allSubnets() will have created two entries (one bare IPv4, one :ipv6).
				// If the lookup misses (e.g. legacy IPv4-only model with only bare IDs),
				// the address is stored without a CIDR tag. This is acceptable for legacy
				// models; new dual-stack machines will always have matching entries.
				if subnetCIDR := subnetIDToCIDR[jujuSubnetID.String()]; subnetCIDR != "" {
					machineAddrOpts = append(machineAddrOpts, network.WithCIDR(subnetCIDR))
				}

			}

			providerAddr := network.NewMachineAddress(
				toValue(ipConf.Properties.PrivateIPAddress),
				machineAddrOpts...,
			).AsProviderAddress(providerAddrOpts...)

			// If this is the primary address ensure that it appears
			// at the top of the address list.
			if isPrimary {
				ni.Addresses = append(network.ProviderAddresses{providerAddr}, ni.Addresses...)
			} else {
				ni.Addresses = append(ni.Addresses, providerAddr)
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

	vnetRG, vnetName := env.networkInfo(context.TODO())
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

func (env *azureEnviron) findSubnet(ctx context.Context, subnetName string) (*azurenetwork.Subnet, error) {
	subnets, err := env.allProviderSubnets(ctx)
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}
	for _, subnet := range subnets {
		if toValue(subnet.Name) == subnetName {
			return subnet, nil
		}
	}
	return nil, errors.NotFoundf("subnet %q", subnetName)
}

// networkInfoForInstance returns the virtual network and subnet to use
// when provisioning an instance.
func (env *azureEnviron) networkInfoForInstance(
	ctx context.Context,
	args environs.StartInstanceParams,
	bootstrapping, controller bool,
	placementSubnet *azurenetwork.Subnet,
) (vnetID string, subnetIDs []network.Id, primarySubnet *azurenetwork.Subnet, _ error) {

	vnetRG, vnetName := env.networkInfo(ctx)
	vnetID = fmt.Sprintf(`[resourceId('Microsoft.Network/virtualNetworks', '%s')]`, vnetName)
	if vnetRG != "" {
		vnetID = fmt.Sprintf(`[resourceId('%s', 'Microsoft.Network/virtualNetworks', '%s')]`, vnetRG, vnetName)
	}

	placementSubnetID := providerSubnetID(placementSubnet)
	isDualStack := args.Constraints.IPFamily != nil && *args.Constraints.IPFamily == ipfamily.Dual
	// When a placement subnet was provided, it is the primary by default.
	// Early returns below may override this, and the space-constraints
	// fallthrough at the bottom uses it to avoid a redundant lookup.
	primarySubnet = placementSubnet

	constraints := args.Constraints

	// We'll collect all the possible subnets and pick one
	// based on constraints and placement.
	var possibleSubnets [][]network.Id

	if !constraints.HasSpaces() {
		// Use placement if specified.
		if placementSubnetID != "" {
			return vnetID, stripAndDeduplicateSubnetIDs([]network.Id{placementSubnetID}), placementSubnet, nil
		}

		// When bootstrapping the network doesn't exist yet so just
		// return the relevant subnet ID and it is created as part of
		// the bootstrap process.
		if bootstrapping && env.config.virtualNetworkName == "" {
			return vnetID, stripAndDeduplicateSubnetIDs([]network.Id{env.defaultControllerSubnet()}), nil, nil
		}

		// Prefer the legacy default subnet if found.
		defaultSubnetName := internalSubnetName
		if controller {
			defaultSubnetName = controllerSubnetName
		}
		defaultSubnet, err := env.findSubnet(ctx, defaultSubnetName)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return "", nil, nil, errors.Trace(err)
		}
		if err == nil {
			return vnetID, stripAndDeduplicateSubnetIDs([]network.Id{providerSubnetID(defaultSubnet)}), defaultSubnet, nil
		}

		// For deployments without a spaces constraint, there's no subnets to zones mapping.
		// So get all accessible subnets.
		allSubnets, err := env.allSubnets(ctx)
		if err != nil {
			return "", nil, nil, env.HandleCredentialError(ctx, err)
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
			return "", nil, nil, errors.Trace(err)
		}
	}

	// For each list of subnet IDs that satisfy space and zone constraints,
	// choose a single one at random.
	var subnetIDForZone []network.Id
	for _, zoneSubnetIDs := range possibleSubnets {
		// Use placement to select a single subnet if needed.
		// Strip the :ipv6 suffix before comparing so that a bare
		// placement ID (e.g. /subnets/foo) matches the suffixed
		// variant (/subnets/foo:ipv6) emitted by allSubnets().
		var subnetIDs []network.Id
		for _, id := range zoneSubnetIDs {
			bareID := network.Id(stripIPFamilySuffix(id.String()))
			if placementSubnetID == "" || placementSubnetID == bareID {
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
		return "", nil, nil, errors.NotFoundf("subnet for constraint %q", constraints.String())
	}

	// Put any placement subnet first in the list
	// so it ia allocated to the primary NIC.
	if placementSubnetID != "" {
		subnetIDs = append(subnetIDs, placementSubnetID)
	}
	for _, id := range subnetIDForZone {
		bareID := network.Id(stripIPFamilySuffix(id.String()))
		if bareID != placementSubnetID {
			subnetIDs = append(subnetIDs, id)
		}
	}

	dedupedSubnetIDs := stripAndDeduplicateSubnetIDs(subnetIDs)

	// Resolve the primary subnet for validation when ip-family=dual.
	if isDualStack && len(dedupedSubnetIDs) > 0 && primarySubnet == nil {
		var err error
		primarySubnet, err = env.findSubnetByID(ctx, dedupedSubnetIDs[0])
		if err != nil && !errors.Is(err, errors.NotFound) {
			return "", nil, nil, errors.Trace(err)
		}
	}

	return vnetID, dedupedSubnetIDs, primarySubnet, nil
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

// providerSubnetID extracts the network.Id from an Azure subnet resource.
func providerSubnetID(subnet *azurenetwork.Subnet) network.Id {
	return network.Id(toValue(toValue(subnet).ID))
}

// findSubnetByID resolves an Azure subnet ID to its full resource.
// It strips the :ipv6 suffix before comparison, so it can look up
// IDs originating from the Juju allSubnets() path.
func (env *azureEnviron) findSubnetByID(ctx context.Context, id network.Id) (*azurenetwork.Subnet, error) {
	subnets, err := env.allProviderSubnets(ctx)
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}
	bare := network.Id(stripIPFamilySuffix(id.String()))
	for _, subnet := range subnets {
		if network.Id(toValue(subnet.ID)) == bare {
			return subnet, nil
		}
	}
	return nil, nil
}

func (env *azureEnviron) findPlacementSubnet(ctx context.Context, placement string) (*azurenetwork.Subnet, error) {
	if placement == "" {
		return nil, nil
	}
	subnetName, err := env.parsePlacement(placement)
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger.Debugf(ctx, "searching for subnet matching placement directive %q", subnetName)
	return env.findSubnet(ctx, subnetName)
}

// subnetProviderIDForFamily builds the Juju provider subnet ID for an Azure subnet,
// optionally suffixed with the IP family. Bare (no suffix) means IPv4; `:ipv6` suffix
// means IPv6. The suffix is used only on the read path (allSubnets, NetworkInterfaces);
// it is stripped before subnet IDs reach ARM templates for provisioning.
func subnetProviderIDForFamily(azureSubnetID string, isIPv6 bool) network.Id {
	if isIPv6 {
		return network.Id(azureSubnetID + ":ipv6")
	}
	return network.Id(azureSubnetID)
}

// stripIPFamilySuffix removes the :ipv4/:ipv6 suffix from a Juju provider subnet ID.
// Non-suffixed (or otherwise-formatted) input is returned unchanged. The colon is safe
// within Juju (only appears in suffixes); Azure ARM resource IDs never contain colons.
func stripIPFamilySuffix(id string) string {
	if idx := strings.LastIndex(id, ":"); idx != -1 {
		switch id[idx+1:] {
		case "ipv4", "ipv6":
			return id[:idx]
		}
	}
	return id
}

// stripAndDeduplicateSubnetIDs removes :ipv6 suffixes from subnet IDs and deduplicates them.
// This ensures that when allSubnets() emits two rows per dual-stack subnet (one for each
// family), we only pass one bare Azure subnet ID to ARM templates. The order of the first
// occurrence is preserved.
func stripAndDeduplicateSubnetIDs(subnetIDs []network.Id) []network.Id {
	seen := make(map[string]bool)
	var result []network.Id
	for _, id := range subnetIDs {
		bare := stripIPFamilySuffix(id.String())
		if !seen[bare] {
			seen[bare] = true
			result = append(result, network.Id(bare))
		}
	}
	return result
}
