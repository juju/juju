// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// Subnets implements environs.NetworkingEnviron.
func (e *environ) Subnets(inst instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	// In GCE all the subnets are in all AZs.
	zones, err := e.AvailabilityZones()
	if err != nil {
		return nil, errors.Trace(err)
	}
	zoneNames := make([]string, len(zones))
	for i, zone := range zones {
		zoneNames[i] = zone.Name()
	}

	ids := makeIncludeSet(subnetIds)
	var results []network.SubnetInfo
	if inst == instance.UnknownId {
		results, err = e.getMatchingSubnets(ids, zoneNames)
	} else {
		results, err = e.getInstanceSubnets(inst, ids, zoneNames)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	if missing := ids.Missing(); len(missing) != 0 {
		return nil, errors.NotFoundf("subnets %v", missing)
	}

	return results, nil
}

func (e *environ) getMatchingSubnets(subnetIds IncludeSet, zones []string) ([]network.SubnetInfo, error) {
	allSubnets, err := e.gce.Subnetworks(e.cloud.Region)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var results []network.SubnetInfo
	for _, subnet := range allSubnets {
		if subnetIds.Include(subnet.SelfLink) {
			results = append(results, makeSubnetInfo(
				network.Id(subnet.SelfLink),
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
	// According to GCE docs, an instance can only have one nic.
	// https://cloud.google.com/compute/docs/instances/#instances_and_networks
	if ifaces := len(googleInst.NetworkInterfaces()); ifaces != 1 {
		return nil, errors.Errorf("expected 1 network interface on %q, found %d", instId, ifaces)
	}
	iface := googleInst.NetworkInterfaces()[0]
	subnetId := network.Id(iface.Subnetwork)
	subnets, err := e.Subnets(instance.UnknownId, []network.Id{subnetId})
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnet := subnets[0]
	results := []network.InterfaceInfo{{
		DeviceIndex: 0,
		CIDR:        subnet.CIDR,
		// XXX(xtian): not sure about this - the network interface has
		// no id in GCE, each machine has exactly one interface, so it
		// can be identified by the machine's id.
		ProviderId:        network.Id(instId),
		ProviderSubnetId:  subnetId,
		AvailabilityZones: subnet.AvailabilityZones,
		InterfaceName:     iface.Name,
		Address:           network.NewScopedAddress(iface.NetworkIP, network.ScopeCloudLocal),
		InterfaceType:     network.EthernetInterface,
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        network.ConfigDHCP,
	}}
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

func makeSubnetInfo(subnetId network.Id, cidr string, zones []string) network.SubnetInfo {
	zonesCopy := make([]string, len(zones))
	copy(zonesCopy, zones)
	return network.SubnetInfo{
		ProviderId:        subnetId,
		CIDR:              cidr,
		AvailabilityZones: zonesCopy,
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
	items := set.NewStrings()
	for _, id := range ids {
		items.Add(string(id))
	}
	return &includeSet{items: items}
}
