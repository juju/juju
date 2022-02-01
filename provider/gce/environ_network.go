// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/gce/google"
)

type subnetMap map[string]corenetwork.SubnetInfo
type networkMap map[string]*compute.Network

// Subnets implements environs.NetworkingEnviron.
func (e *environ) Subnets(
	ctx context.ProviderCallContext, inst instance.Id, subnetIds []corenetwork.Id,
) ([]corenetwork.SubnetInfo, error) {
	// In GCE all the subnets are in all AZs.
	zones, err := e.zoneNames(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ids := makeIncludeSet(subnetIds)
	var results []corenetwork.SubnetInfo
	if inst == instance.UnknownId {
		results, err = e.getMatchingSubnets(ctx, ids, zones)
	} else {
		results, err = e.getInstanceSubnets(ctx, inst, ids, zones)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	if missing := ids.Missing(); len(missing) != 0 {
		return nil, errors.NotFoundf("subnets %v", formatMissing(missing))
	}

	return results, nil
}

func (e *environ) zoneNames(ctx context.ProviderCallContext) ([]string, error) {
	zones, err := e.AvailabilityZones(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	names := make([]string, len(zones))
	for i, zone := range zones {
		names[i] = zone.Name()
	}
	return names, nil
}

func (e *environ) networksByURL(ctx context.ProviderCallContext) (networkMap, error) {
	networks, err := e.gce.Networks()
	if err != nil {
		return nil, google.HandleCredentialError(errors.Trace(err), ctx)
	}
	results := make(networkMap)
	for _, network := range networks {
		results[network.SelfLink] = network
	}
	return results, nil
}

func (e *environ) getMatchingSubnets(
	ctx context.ProviderCallContext, subnetIds IncludeSet, zones []string,
) ([]corenetwork.SubnetInfo, error) {
	allSubnets, err := e.gce.Subnetworks(e.cloud.Region)
	if err != nil {
		return nil, google.HandleCredentialError(errors.Trace(err), ctx)
	}
	networks, err := e.networksByURL(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var results []corenetwork.SubnetInfo
	for _, subnet := range allSubnets {
		netwk, ok := networks[subnet.Network]
		if !ok {
			return nil, errors.NotFoundf("network %q for subnet %q", subnet.Network, subnet.Name)
		}
		if subnetIds.Include(subnet.Name) {
			results = append(results, makeSubnetInfo(
				corenetwork.Id(subnet.Name),
				corenetwork.Id(netwk.Name),
				subnet.IpCidrRange,
				zones,
			))
		}
	}
	// We have to include networks in 'LEGACY' mode that do not have subnetworks.
	for _, netwk := range networks {
		if netwk.IPv4Range != "" && subnetIds.Include(netwk.Name) {
			results = append(results, makeSubnetInfo(
				corenetwork.Id(netwk.Name),
				corenetwork.Id(netwk.Name),
				netwk.IPv4Range,
				zones,
			))
		}
	}
	return results, nil
}

func (e *environ) getInstanceSubnets(
	ctx context.ProviderCallContext, inst instance.Id, subnetIds IncludeSet, zones []string,
) ([]corenetwork.SubnetInfo, error) {
	ifLists, err := e.NetworkInterfaces(ctx, []instance.Id{inst})
	if err != nil {
		return nil, errors.Trace(err)
	}
	ifaces := ifLists[0]

	var results []corenetwork.SubnetInfo
	for _, iface := range ifaces {
		if subnetIds.Include(string(iface.ProviderSubnetId)) {
			results = append(results, makeSubnetInfo(
				iface.ProviderSubnetId,
				iface.ProviderNetworkId,
				iface.PrimaryAddress().CIDR,
				zones,
			))
		}
	}
	return results, nil
}

// NetworkInterfaces implements environs.NetworkingEnviron.
func (e *environ) NetworkInterfaces(ctx context.ProviderCallContext, ids []instance.Id) ([]corenetwork.InterfaceInfos, error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}

	// Fetch instance information for the IDs we are interested in.
	insts, err := e.Instances(ctx, ids)
	partialInfo := err == environs.ErrPartialInstances
	if err != nil && err != environs.ErrPartialInstances {
		if errors.Cause(err) == environs.ErrNoInstances {
			return nil, err
		}
		return nil, errors.Trace(err)
	}

	// In GCE all the subnets are in all AZs.
	zones, err := e.zoneNames(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	networks, err := e.networksByURL(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Extract the unique list of subnet URLs we are interested in for
	// all instances and fetch related subnet information.
	uniqueSubnetURLs, err := getUniqueSubnetURLs(ids, insts)
	if err != nil {
		return nil, errors.Trace(err)
	}

	subnets, err := e.subnetsByURL(ctx, uniqueSubnetURLs.Values(), networks, zones)
	if err != nil {
		return nil, errors.Trace(err)
	}

	infos := make([]corenetwork.InterfaceInfos, len(ids))
	for idx, inst := range insts {
		if inst == nil {
			continue // no instance with this ID known by provider
		}

		// Note: we have already verified that we can safely cast inst
		// to environInstance when we iterated the instance list to
		// obtain the unique subnet URLs
		for i, iface := range inst.(*environInstance).base.NetworkInterfaces() {
			details, err := findNetworkDetails(iface, subnets, networks)
			if err != nil {
				return nil, errors.Annotatef(err, "instance %q", ids[idx])
			}

			// Scan the access configs for public addresses
			var shadowAddrs corenetwork.ProviderAddresses
			for _, accessConf := range iface.AccessConfigs {
				// According to the gce docs only ONE_TO_ONE_NAT
				// is currently supported for external IPs
				if accessConf.Type != "ONE_TO_ONE_NAT" {
					continue
				}

				shadowAddrs = append(shadowAddrs,
					corenetwork.NewMachineAddress(accessConf.NatIP, corenetwork.WithScope(corenetwork.ScopePublic)).AsProviderAddress(),
				)
			}

			infos[idx] = append(infos[idx], corenetwork.InterfaceInfo{
				DeviceIndex: i,
				// The network interface has no id in GCE so it's
				// identified by the machine's id + its name.
				ProviderId:        corenetwork.Id(fmt.Sprintf("%s/%s", ids[idx], iface.Name)),
				ProviderSubnetId:  details.subnet,
				ProviderNetworkId: details.network,
				AvailabilityZones: copyStrings(zones),
				InterfaceName:     iface.Name,
				Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
					iface.NetworkIP,
					corenetwork.WithScope(corenetwork.ScopeCloudLocal),
					corenetwork.WithCIDR(details.cidr),
					corenetwork.WithConfigType(corenetwork.ConfigDHCP),
				).AsProviderAddress()},
				ShadowAddresses: shadowAddrs,
				InterfaceType:   corenetwork.EthernetDevice,
				Disabled:        false,
				NoAutoStart:     false,
				Origin:          corenetwork.OriginProvider,
			})
		}
	}

	if partialInfo {
		err = environs.ErrPartialInstances
	}
	return infos, err
}

func getUniqueSubnetURLs(ids []instance.Id, insts []instances.Instance) (set.Strings, error) {
	uniqueSet := set.NewStrings()

	for idx, inst := range insts {
		if inst == nil {
			continue // no instance with this ID known by provider
		}

		envInst, ok := inst.(*environInstance)
		if !ok { // This shouldn't happen.
			return nil, errors.Errorf("couldn't extract GCE instance for %q", ids[idx])
		}

		for _, iface := range envInst.base.NetworkInterfaces() {
			if iface.Subnetwork != "" {
				uniqueSet.Add(iface.Subnetwork)
			}
		}
	}

	return uniqueSet, nil
}

type networkDetails struct {
	cidr    string
	subnet  corenetwork.Id
	network corenetwork.Id
}

// findNetworkDetails looks up the network information we need to
// populate an InterfaceInfo - if the interface is on a legacy network
// we use information from the network because there'll be no subnet
// linked.
func findNetworkDetails(iface compute.NetworkInterface, subnets subnetMap, networks networkMap) (networkDetails, error) {
	var result networkDetails
	if iface.Subnetwork == "" {
		// This interface is on a legacy network.
		netwk, ok := networks[iface.Network]
		if !ok {
			return result, errors.NotFoundf("network %q", iface.Network)
		}
		result.cidr = netwk.IPv4Range
		result.subnet = ""
		result.network = corenetwork.Id(netwk.Name)
	} else {
		subnet, ok := subnets[iface.Subnetwork]
		if !ok {
			return result, errors.NotFoundf("subnet %q", iface.Subnetwork)
		}
		result.cidr = subnet.CIDR
		result.subnet = subnet.ProviderId
		result.network = subnet.ProviderNetworkId
	}
	return result, nil
}

func (e *environ) subnetsByURL(ctx context.ProviderCallContext, urls []string, networks networkMap, zones []string) (subnetMap, error) {
	if len(urls) == 0 {
		return make(map[string]corenetwork.SubnetInfo), nil
	}
	urlSet := includeSet{items: set.NewStrings(urls...)}
	allSubnets, err := e.gce.Subnetworks(e.cloud.Region)
	if err != nil {
		return nil, google.HandleCredentialError(errors.Trace(err), ctx)
	}
	results := make(map[string]corenetwork.SubnetInfo)
	for _, subnet := range allSubnets {
		netwk, ok := networks[subnet.Network]
		if !ok {
			return nil, errors.NotFoundf("network %q for subnet %q", subnet.Network, subnet.Name)
		}
		if urlSet.Include(subnet.SelfLink) {
			results[subnet.SelfLink] = makeSubnetInfo(
				corenetwork.Id(subnet.Name),
				corenetwork.Id(netwk.Name),
				subnet.IpCidrRange,
				zones,
			)
		}
	}
	if missing := urlSet.Missing(); len(missing) != 0 {
		return nil, errors.NotFoundf("subnets %v", formatMissing(missing))
	}
	return results, nil
}

// SupportsSpaces implements environs.NetworkingEnviron.
func (e *environ) SupportsSpaces(ctx context.ProviderCallContext) (bool, error) {
	return false, nil
}

// SupportsContainerAddresses implements environs.NetworkingEnviron.
func (e *environ) SupportsContainerAddresses(ctx context.ProviderCallContext) (bool, error) {
	return false, nil
}

// AllocateContainerAddresses implements environs.NetworkingEnviron.
func (e *environ) AllocateContainerAddresses(context.ProviderCallContext, instance.Id, names.MachineTag, corenetwork.InterfaceInfos) (corenetwork.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("container addresses")
}

// ReleaseContainerAddresses implements environs.NetworkingEnviron.
func (e *environ) ReleaseContainerAddresses(context.ProviderCallContext, []corenetwork.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container addresses")
}

// AreSpacesRoutable implements environs.NetworkingEnviron.
func (*environ) AreSpacesRoutable(ctx context.ProviderCallContext, space1, space2 *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// SSHAddresses implements environs.SSHAddresses.
// For GCE we want to make sure we're returning only one public address, so that probing won't
// cause SSHGuard to lock us out
func (*environ) SSHAddresses(ctx context.ProviderCallContext, addresses corenetwork.SpaceAddresses) (corenetwork.SpaceAddresses, error) {
	bestAddress, ok := addresses.OneMatchingScope(corenetwork.ScopeMatchPublic)
	if ok {
		return corenetwork.SpaceAddresses{bestAddress}, nil
	} else {
		// fallback
		return addresses, nil
	}
}

// SuperSubnets implements environs.SuperSubnets
func (e *environ) SuperSubnets(ctx context.ProviderCallContext) ([]string, error) {
	subnets, err := e.Subnets(ctx, "", nil)
	if err != nil {
		return nil, err
	}
	cidrs := make([]string, len(subnets))
	for i, subnet := range subnets {
		cidrs[i] = subnet.CIDR
	}
	return cidrs, nil
}

func copyStrings(items []string) []string {
	if items == nil {
		return nil
	}
	result := make([]string, len(items))
	copy(result, items)
	return result
}

func makeSubnetInfo(
	subnetId corenetwork.Id, networkId corenetwork.Id, cidr string, zones []string,
) corenetwork.SubnetInfo {
	return corenetwork.SubnetInfo{
		ProviderId:        subnetId,
		ProviderNetworkId: networkId,
		CIDR:              cidr,
		AvailabilityZones: copyStrings(zones),
		VLANTag:           0,
	}
}

// IncludeSet represents a set of items that can be crossed off once,
// and when you're finished crossing items off then you can see what's
// left.
type IncludeSet interface {
	// Include returns whether this item should be included, and
	// crosses it off.
	Include(item string) bool
	// Missing returns any items that haven't been crossed off (as a
	// sorted slice).
	Missing() []string
}

// includeAny allows any items and doesn't report any as missing.
type includeAny struct{}

// Include implements IncludeSet.
func (includeAny) Include(string) bool { return true }

// Missing implements IncludeSet.
func (includeAny) Missing() []string { return nil }

// includeSet is a set of items that we want to find in some results.
type includeSet struct {
	items set.Strings
}

// Include implements IncludeSet.
func (s *includeSet) Include(item string) bool {
	if s.items.Contains(item) {
		s.items.Remove(item)
		return true
	}
	return false
}

// Missing implements IncludeSet.
func (s *includeSet) Missing() []string {
	return s.items.SortedValues()
}

func makeIncludeSet(ids []corenetwork.Id) IncludeSet {
	if len(ids) == 0 {
		return &includeAny{}
	}
	str := set.NewStrings()
	for _, id := range ids {
		str.Add(string(id))
	}
	return &includeSet{items: str}
}

func formatMissing(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = fmt.Sprintf("%q", item)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
