// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"google.golang.org/api/compute/v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type subnetMap map[string]network.SubnetInfo
type networkMap map[string]*compute.Network

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
		return nil, errors.NotFoundf("subnets %v", formatMissing(missing))
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

func (e *environ) networksByURL() (networkMap, error) {
	networks, err := e.gce.Networks()
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make(networkMap)
	for _, network := range networks {
		results[network.SelfLink] = network
	}
	return results, nil
}

func (e *environ) getMatchingSubnets(subnetIds IncludeSet, zones []string) ([]network.SubnetInfo, error) {
	allSubnets, err := e.gce.Subnetworks(e.cloud.Region)
	if err != nil {
		return nil, errors.Trace(err)
	}
	networks, err := e.networksByURL()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var results []network.SubnetInfo
	for _, subnet := range allSubnets {
		netwk, ok := networks[subnet.Network]
		if !ok {
			return nil, errors.NotFoundf("network %q for subnet %q", subnet.Network, subnet.Name)
		}
		if subnetIds.Include(subnet.Name) {
			results = append(results, makeSubnetInfo(
				network.Id(subnet.Name),
				network.Id(netwk.Name),
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
	// In GCE all the subnets are in all AZs.
	zones, err := e.zoneNames()
	if err != nil {
		return nil, errors.Trace(err)
	}
	networks, err := e.networksByURL()
	if err != nil {
		return nil, errors.Trace(err)
	}
	googleInst := envInst.base
	ifaces := googleInst.NetworkInterfaces()

	var subnetURLs []string
	for _, iface := range ifaces {
		if iface.Subnetwork != "" {
			subnetURLs = append(subnetURLs, iface.Subnetwork)
		}
	}
	subnets, err := e.subnetsByURL(subnetURLs, networks, zones)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We know there'll be a subnet for each url requested, otherwise
	// there would have been an error.

	var results []network.InterfaceInfo
	for i, iface := range ifaces {
		details, err := findNetworkDetails(iface, subnets, networks)
		if err != nil {
			return nil, errors.Annotatef(err, "instance %q", instId)
		}
		results = append(results, network.InterfaceInfo{
			DeviceIndex: i,
			CIDR:        details.cidr,
			// The network interface has no id in GCE so it's
			// identified by the machine's id + its name.
			ProviderId:        network.Id(fmt.Sprintf("%s/%s", instId, iface.Name)),
			ProviderSubnetId:  details.subnet,
			ProviderNetworkId: details.network,
			AvailabilityZones: copyStrings(zones),
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

type networkDetails struct {
	cidr    string
	subnet  network.Id
	network network.Id
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
		result.network = network.Id(netwk.Name)
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

func (e *environ) subnetsByURL(urls []string, networks networkMap, zones []string) (subnetMap, error) {
	if len(urls) == 0 {
		return make(map[string]network.SubnetInfo), nil
	}
	urlSet := includeSet{items: set.NewStrings(urls...)}
	allSubnets, err := e.gce.Subnetworks(e.cloud.Region)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make(map[string]network.SubnetInfo)
	for _, subnet := range allSubnets {
		netwk, ok := networks[subnet.Network]
		if !ok {
			return nil, errors.NotFoundf("network %q for subnet %q", subnet.Network, subnet.Name)
		}
		if urlSet.Include(subnet.SelfLink) {
			results[subnet.SelfLink] = makeSubnetInfo(
				network.Id(subnet.Name),
				network.Id(netwk.Name),
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

// ProviderSpaceInfo implements environs.NetworkingEnviron.
func (*environ) ProviderSpaceInfo(space *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// AreSpacesRoutable implements environs.NetworkingEnviron.
func (*environ) AreSpacesRoutable(space1, space2 *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// SSHAddresses implements environs.SSHAddresses.
// For GCE we want to make sure we're returning only one public address, so that probing won't
// cause SSHGuard to lock us out
func (*environ) SSHAddresses(addresses []network.Address) ([]network.Address, error) {
	bestAddress, ok := network.SelectPublicAddress(addresses)
	if ok {
		return []network.Address{bestAddress}, nil
	} else {
		// fallback
		return addresses, nil
	}
}

// SuperSubnets implements environs.SuperSubnets
func (*environ) SuperSubnets() ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

func copyStrings(items []string) []string {
	if items == nil {
		return nil
	}
	result := make([]string, len(items))
	copy(result, items)
	return result
}

func makeSubnetInfo(subnetId network.Id, networkId network.Id, cidr string, zones []string) network.SubnetInfo {
	return network.SubnetInfo{
		ProviderId:        subnetId,
		ProviderNetworkId: networkId,
		CIDR:              cidr,
		AvailabilityZones: copyStrings(zones),
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

func formatMissing(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = fmt.Sprintf("%q", item)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
