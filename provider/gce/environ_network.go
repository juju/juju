// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// Subnets implements environs.NetworkingEnviron.
func (e *environ) Subnets(inst instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	// In GCE all the subnets are in all AZs.
	zones, err := e.zoneNames()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ids := makeIncludeSet(subnetIds)
	var results []network.SubnetInfo
	if inst == instance.UnknownId {
		results, err = e.getMatchingSubnets(ids, zones)
	} else {
		results, err = e.getInstanceSubnets(inst, ids, zones)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	if missing := ids.Missing(); len(missing) != 0 {
		return nil, errors.NotFoundf("subnets %v", missing)
	}

	return results, nil
}

func (e *environ) zoneNames() ([]string, error) {
	zones, err := e.AvailabilityZones()
	if err != nil {
		return nil, errors.Trace(err)
	}
	names := make([]string, len(zones))
	for i, zone := range zones {
		names[i] = zone.Name()
	}
	return names, nil
}

func (e *environ) networkIdsByURL() (map[string]string, error) {
	networks, err := e.gce.Networks()
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make(map[string]string)
	for _, network := range networks {
		results[network.SelfLink] = network.Name
	}
	return results, nil
}

func (e *environ) getMatchingSubnets(subnetIds IncludeSet, zones []string) ([]network.SubnetInfo, error) {
	allSubnets, err := e.gce.Subnetworks(e.cloud.Region)
	if err != nil {
		return nil, errors.Trace(err)
	}
	networks, err := e.networkIdsByURL()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var results []network.SubnetInfo
	for _, subnet := range allSubnets {
		networkId, ok := networks[subnet.Network]
		if !ok {
			return nil, errors.NotFoundf("network %q for subnet %q", subnet.Network, subnet.Name)
		}
		if subnetIds.Include(subnet.Name) {
			results = append(results, makeSubnetInfo(
				network.Id(subnet.Name),
				network.Id(networkId),
				subnet.IpCidrRange,
				zones,
			))
		}
	}
	return results, nil
}

func (e *environ) getInstanceSubnets(inst instance.Id, subnetIds IncludeSet, zones []string) ([]network.SubnetInfo, error) {
	ifaces, err := e.NetworkInterfaces(inst)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var results []network.SubnetInfo
	for _, iface := range ifaces {
		if subnetIds.Include(string(iface.ProviderSubnetId)) {
			results = append(results, makeSubnetInfo(
				iface.ProviderSubnetId,
				iface.ProviderNetworkId,
				iface.CIDR,
				zones,
			))
		}
	}
	return results, nil
}

// NetworkInterfaces implements environs.NetworkingEnviron.
func (e *environ) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	insts, err := e.Instances([]instance.Id{instId})
	if err != nil {
		return nil, errors.Trace(err)
	}
	envInst, ok := insts[0].(*environInstance)
	if !ok {
		// This shouldn't happen.
		return nil, errors.Errorf("couldn't extract google instance for %q", instId)
	}
	googleInst := envInst.base
	ifaces := googleInst.NetworkInterfaces()

	subnetURLs := make([]string, len(ifaces))
	for i, iface := range ifaces {
		subnetURLs[i] = iface.Subnetwork
	}
	subnets, err := e.subnetsByURL(subnetURLs...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We know there'll be a subnet for each url requested, otherwise
	// there would have been an error.

	var results []network.InterfaceInfo
	for i, iface := range ifaces {
		subnet, ok := subnets[iface.Subnetwork]
		if !ok {
			// Should never happen.
			return nil, errors.Errorf("no subnet %q found for instance %q", iface.Subnetwork, instId)
		}

		results = append(results, network.InterfaceInfo{
			DeviceIndex: i,
			CIDR:        subnet.CIDR,
			// The network interface has no id in GCE so it's
			// identified by the machine's id + its name.
			ProviderId:        network.Id(fmt.Sprintf("%s/%s", instId, iface.Name)),
			ProviderSubnetId:  subnet.ProviderId,
			ProviderNetworkId: subnet.ProviderNetworkId,
			AvailabilityZones: subnet.AvailabilityZones,
			InterfaceName:     iface.Name,
			Address:           network.NewScopedAddress(iface.NetworkIP, network.ScopeCloudLocal),
			InterfaceType:     network.EthernetInterface,
			Disabled:          false,
			NoAutoStart:       false,
			ConfigType:        network.ConfigDHCP,
		})
	}
	return results, nil
}

func (e *environ) subnetsByURL(urls ...string) (map[string]network.SubnetInfo, error) {
	if len(urls) == 0 {
		return make(map[string]network.SubnetInfo), nil
	}
	// In GCE all the subnets are in all AZs.
	zones, err := e.zoneNames()
	if err != nil {
		return nil, errors.Trace(err)
	}
	networks, err := e.networkIdsByURL()
	if err != nil {
		return nil, errors.Trace(err)
	}
	urlSet := includeSet{items: set.NewStrings(urls...)}
	allSubnets, err := e.gce.Subnetworks(e.cloud.Region)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make(map[string]network.SubnetInfo)
	for _, subnet := range allSubnets {
		networkId, ok := networks[subnet.Network]
		if !ok {
			return nil, errors.NotFoundf("network %q for subnet %q", subnet.Network, subnet.Name)
		}
		if urlSet.Include(subnet.SelfLink) {
			results[subnet.SelfLink] = makeSubnetInfo(
				network.Id(subnet.Name),
				network.Id(networkId),
				subnet.IpCidrRange,
				zones,
			)
		}
	}
	if missing := urlSet.Missing(); len(missing) != 0 {
		return nil, errors.NotFoundf("subnets %v", missing)
	}
	return results, nil
}

// SupportsSpaces implements environs.NetworkingEnviron.
func (e *environ) SupportsSpaces() (bool, error) {
	return false, nil
}

// SupportsSpaceDiscovery implements environs.NetworkingEnviron.
func (e *environ) SupportsSpaceDiscovery() (bool, error) {
	return false, nil
}

// Spaces implements environs.NetworkingEnviron.
func (e *environ) Spaces() ([]network.SpaceInfo, error) {
	return nil, errors.NotSupportedf("spaces")
}

// SupportsContainerAddresses implements environs.NetworkingEnviron.
func (e *environ) SupportsContainerAddresses() (bool, error) {
	return false, nil
}

// AllocateContainerAddresses implements environs.NetworkingEnviron.
func (e *environ) AllocateContainerAddresses(instance.Id, names.MachineTag, []network.InterfaceInfo) ([]network.InterfaceInfo, error) {
	return nil, errors.NotSupportedf("container addresses")
}

// ReleaseContainerAddresses implements environs.NetworkingEnviron.
func (e *environ) ReleaseContainerAddresses([]network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container addresses")
}

func makeSubnetInfo(subnetId network.Id, networkId network.Id, cidr string, zones []string) network.SubnetInfo {
	zonesCopy := make([]string, len(zones))
	copy(zonesCopy, zones)
	return network.SubnetInfo{
		ProviderId:        subnetId,
		ProviderNetworkId: networkId,
		CIDR:              cidr,
		AvailabilityZones: zonesCopy,
		VLANTag:           0,
		SpaceProviderId:   "",
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

func makeIncludeSet(ids []network.Id) IncludeSet {
	if len(ids) == 0 {
		return &includeAny{}
	}
	strings := set.NewStrings()
	for _, id := range ids {
		strings.Add(string(id))
	}
	return &includeSet{items: strings}
}
