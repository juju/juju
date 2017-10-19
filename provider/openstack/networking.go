// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"net"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"gopkg.in/goose.v2/neutron"
	"gopkg.in/goose.v2/nova"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// Networking is an interface providing networking-related operations
// for an OpenStack Environ.
type Networking interface {
	// AllocatePublicIP allocates a public (floating) IP
	// to the specified instance.
	AllocatePublicIP(instance.Id) (*string, error)

	// DefaultNetworks returns the set of networks that should be
	// added by default to all new instances.
	DefaultNetworks() ([]nova.ServerNetworks, error)

	// ResolveNetwork takes either a network ID or label
	// with a string to specify whether the network is external
	// and returns the corresponding network ID.
	ResolveNetwork(string, bool) (string, error)

	// Subnets returns basic information about subnets known
	// by OpenStack for the environment.
	// Needed for Environ.Networking
	Subnets(instance.Id, []network.Id) ([]network.SubnetInfo, error)

	// NetworkInterfaces requests information about the network
	// interfaces on the given instance.
	// Needed for Environ.Networking
	NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error)
}

// NetworkingDecorator is an interface that provides a means of overriding
// the default Networking implementation.
type NetworkingDecorator interface {
	// DecorateNetworking can be used to return a new Networking
	// implementation that overrides the provided, default Networking
	// implementation.
	DecorateNetworking(Networking) (Networking, error)
}

// switchingNetworking is an implementation of Networking that delegates
// to either Neutron networking (preferred), or legacy Nova networking if
// there is no support for Neutron.
type switchingNetworking struct {
	env *Environ

	mu         sync.Mutex
	networking Networking
}

func (n *switchingNetworking) initNetworking() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.networking != nil {
		return nil
	}

	client := n.env.client()
	if !client.IsAuthenticated() {
		if err := authenticateClient(client); err != nil {
			return errors.Trace(err)
		}
	}

	base := networkingBase{env: n.env}
	if n.env.supportsNeutron() {
		n.networking = &NeutronNetworking{base}
	} else {
		n.networking = &LegacyNovaNetworking{base}
	}
	return nil
}

// AllocatePublicIP is part of the Networking interface.
func (n *switchingNetworking) AllocatePublicIP(instId instance.Id) (*string, error) {
	if err := n.initNetworking(); err != nil {
		return nil, errors.Trace(err)
	}
	return n.networking.AllocatePublicIP(instId)
}

// DefaultNetworks is part of the Networking interface.
func (n *switchingNetworking) DefaultNetworks() ([]nova.ServerNetworks, error) {
	if err := n.initNetworking(); err != nil {
		return nil, errors.Trace(err)
	}
	return n.networking.DefaultNetworks()
}

// ResolveNetwork is part of the Networking interface.
func (n *switchingNetworking) ResolveNetwork(name string, external bool) (string, error) {
	if err := n.initNetworking(); err != nil {
		return "", errors.Trace(err)
	}
	return n.networking.ResolveNetwork(name, external)
}

// Subnets is part of the Networking interface.
func (n *switchingNetworking) Subnets(instId instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	if err := n.initNetworking(); err != nil {
		return nil, errors.Trace(err)
	}
	return n.networking.Subnets(instId, subnetIds)
}

// NetworkInterfaces is part of the Networking interface
func (n *switchingNetworking) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	if err := n.initNetworking(); err != nil {
		return nil, errors.Trace(err)
	}
	return n.networking.NetworkInterfaces(instId)
}

type networkingBase struct {
	env *Environ
}

func processResolveNetworkIds(name string, networkIds []string) (string, error) {
	switch len(networkIds) {
	case 1:
		return networkIds[0], nil
	case 0:
		return "", errors.Errorf("no networks exist with label %q", name)
	}
	return "", errors.Errorf("multiple networks with label %q: %v", name, networkIds)
}

// NeutronNetworking is an implementation of Networking that uses the Neutron
// network APIs.
type NeutronNetworking struct {
	networkingBase
}

// AllocatePublicIP is part of the Networking interface.
func (n *NeutronNetworking) AllocatePublicIP(instId instance.Id) (*string, error) {
	extNetworkIds := make([]string, 0)
	neutronClient := n.env.neutron()
	externalNetwork := n.env.ecfg().externalNetwork()
	if externalNetwork != "" {
		// the config specified an external network, try it first.
		netId, err := resolveNeutronNetwork(neutronClient, externalNetwork, true)
		if err != nil {
			logger.Debugf("external network %s not found, search for one", externalNetwork)
		} else {
			logger.Debugf("using external network %q", externalNetwork)
			extNetworkIds = []string{netId}
		}
	}

	if len(extNetworkIds) == 0 {
		// Create slice of network.Ids for external networks in the same AZ as
		// the instance's network, to find an existing floating ip in, or allocate
		// a new floating ip from.
		network := n.env.ecfg().network()
		netId, err := resolveNeutronNetwork(neutronClient, network, false)
		netDetails, err := neutronClient.GetNetworkV2(netId)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, az := range netDetails.AvailabilityZones {
			extNetIds, _ := getExternalNeutronNetworksByAZ(n.env, az)
			if len(extNetIds) > 0 {
				extNetworkIds = append(extNetworkIds, extNetIds...)
			}
		}

		if len(extNetworkIds) == 0 {
			return nil, errors.NewNotFound(nil, fmt.Sprintf("could not find an external network in availability zone %s", netDetails.AvailabilityZones))
		}
	}

	fips, err := n.env.neutron().ListFloatingIPsV2()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Is there an unused FloatingIP on an external network in the instance's availability zone?
	for _, fip := range fips {
		if fip.FixedIP == "" {
			// Not a perfect solution.  If an external network was specified in the
			// config, it'll be at the top of the extNetworkIds, but may be not used
			// if the available FIP isn't it in.  However the instance and the
			// FIP will be in the same availability zone.
			for _, extNetId := range extNetworkIds {
				if fip.FloatingNetworkId == extNetId {
					logger.Debugf("found unassigned public ip: %v", fip.IP)
					return &fip.IP, nil
				}
			}
		}
	}

	// allocate a new IP and use it
	var lastErr error
	for _, extNetId := range extNetworkIds {
		var newfip *neutron.FloatingIPV2
		newfip, lastErr = neutronClient.AllocateFloatingIPV2(extNetId)
		if lastErr == nil {
			logger.Debugf("allocated new public IP: %s", newfip.IP)
			return &newfip.IP, nil
		}
	}

	logger.Debugf("Unable to allocate a public IP")
	return nil, lastErr
}

// externalNetworkFilter returns a neutron.Filter to match Neutron Networks with
// router:external = true.
func externalNetworkFilter() *neutron.Filter {
	filter := neutron.NewFilter()
	filter.Set(neutron.FilterRouterExternal, "true")
	return filter
}

// getExternalNeutronNetworksByAZ returns all external networks within the
// given availability zone. If azName is empty, return all external networks.
func getExternalNeutronNetworksByAZ(e *Environ, azName string) ([]string, error) {
	neutron := e.neutron()
	// Find all external networks in availability zone
	networks, err := neutron.ListNetworksV2(externalNetworkFilter())
	if err != nil {
		return nil, errors.Trace(err)
	}
	netIds := make([]string, 0)
	for _, network := range networks {
		// TODO (hml): OpenStack Compute and Network AZs have no direct relation,
		// though they can be named the same.  The default AZ is named "nova" in
		// either case. It's possible that a compute AZ was configured but not a
		// network one.  Need to account for this.
		for _, netAZ := range network.AvailabilityZones {
			if azName == netAZ {
				netIds = append(netIds, network.Id)
				break
			}
		}
	}
	if len(netIds) == 0 {
		return nil, errors.NewNotFound(nil, "No External networks found to allocate a Floating IP")
	}
	return netIds, nil
}

// DefaultNetworks is part of the Networking interface.
func (n *NeutronNetworking) DefaultNetworks() ([]nova.ServerNetworks, error) {
	return []nova.ServerNetworks{}, nil
}

// ResolveNetwork is part of the Networking interface.
func (n *NeutronNetworking) ResolveNetwork(name string, external bool) (string, error) {
	return resolveNeutronNetwork(n.env.neutron(), name, external)
}

// networkFilter returns a neutron.Filter to match Neutron Networks with
// the exact given name AND router:external boolean result.
func networkFilter(name string, external bool) *neutron.Filter {
	filter := neutron.NewFilter()
	filter.Set(neutron.FilterNetwork, fmt.Sprintf("%s", name))
	filter.Set(neutron.FilterRouterExternal, fmt.Sprintf("%t", external))
	return filter
}

func resolveNeutronNetwork(neutron *neutron.Client, name string, external bool) (string, error) {
	if utils.IsValidUUIDString(name) {
		return name, nil
	}
	networks, err := neutron.ListNetworksV2(networkFilter(name, external))
	if err != nil {
		return "", err
	}
	var networkIds []string
	for _, network := range networks {
		networkIds = append(networkIds, network.Id)
	}
	return processResolveNetworkIds(name, networkIds)
}

func makeSubnetInfo(neutron *neutron.Client, subnet neutron.SubnetV2) (network.SubnetInfo, error) {
	_, _, err := net.ParseCIDR(subnet.Cidr)
	if err != nil {
		return network.SubnetInfo{}, errors.Annotatef(err, "skipping subnet %q, invalid CIDR", subnet.Cidr)
	}
	net, err := neutron.GetNetworkV2(subnet.NetworkId)
	if err != nil {
		return network.SubnetInfo{}, err
	}

	// TODO (hml) 2017-03-20:
	// With goose updates, VLANTag can be updated to be
	// network.segmentation_id, if network.network_type equals vlan
	info := network.SubnetInfo{
		CIDR:              subnet.Cidr,
		ProviderId:        network.Id(subnet.Id),
		VLANTag:           0,
		AvailabilityZones: net.AvailabilityZones,
		SpaceProviderId:   "",
	}
	logger.Tracef("found subnet with info %#v", info)
	return info, nil
}

// Subnets returns basic information about the specified subnets known
// by the provider for the specified instance or list of ids. subnetIds can be
// empty, in which case all known are returned.
func (n *NeutronNetworking) Subnets(instId instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	var results []network.SubnetInfo
	subIdSet := set.NewStrings()
	for _, subId := range subnetIds {
		subIdSet.Add(string(subId))
	}

	if instId != instance.UnknownId {
		// TODO(hml): 2017-03-20
		// Implement Subnets() for case where instId is specified
		return nil, errors.NotSupportedf("neutron subnets with instance Id")
	} else {
		neutron := n.env.neutron()
		subnets, err := neutron.ListSubnetsV2()
		if err != nil {
			return nil, errors.Annotatef(err, "failed to retrieve subnets")
		}
		if len(subnetIds) == 0 {
			for _, subnet := range subnets {
				subIdSet.Add(string(subnet.Id))
			}
		}
		for _, subnet := range subnets {
			if !subIdSet.Contains(string(subnet.Id)) {
				logger.Tracef("subnet %q not in %v, skipping", subnet.Id, subnetIds)
				continue
			}
			subIdSet.Remove(string(subnet.Id))
			if info, err := makeSubnetInfo(neutron, subnet); err == nil {
				// Error will already have been logged.
				results = append(results, info)
			}

		}
	}
	if !subIdSet.IsEmpty() {
		return nil, errors.Errorf("failed to find the following subnet ids: %v", subIdSet.Values())
	}
	return results, nil
}

func (n *NeutronNetworking) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	return nil, errors.NotSupportedf("neutron network interfaces")
}
