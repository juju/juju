// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/goose.v2/nova"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
)

// LegacyNovaNetworking is an implementation of Networking that uses the legacy
// Nova network APIs.
//
// NOTE(axw) this is provided on a best-effort basis, primarily for CI testing
// of Juju until we are no longer dependent on an old OpenStack installation.
// This should not be relied on in production, and should be removed as soon as
// possible.
type LegacyNovaNetworking struct {
	networkingBase
}

// AllocatePublicIP is part of the Networking interface.
func (n *LegacyNovaNetworking) AllocatePublicIP(instId instance.Id) (*string, error) {
	fips, err := n.env.nova().ListFloatingIPs()
	if err != nil {
		return nil, err
	}
	var newfip *nova.FloatingIP
	for _, fip := range fips {
		newfip = &fip
		if fip.InstanceId != nil && *fip.InstanceId != "" {
			// unavailable, skip
			newfip = nil
			continue
		} else {
			logger.Debugf("found unassigned public ip: %v", newfip.IP)
			// unassigned, we can use it
			return &newfip.IP, nil
		}
	}
	if newfip == nil {
		// allocate a new IP and use it
		newfip, err = n.env.nova().AllocateFloatingIP()
		if err != nil {
			return nil, err
		}
		logger.Debugf("allocated new public IP: %v", newfip.IP)
	}
	return &newfip.IP, nil
}

// DefaultNetworks is part of the Networking interface.
func (*LegacyNovaNetworking) DefaultNetworks() ([]nova.ServerNetworks, error) {
	return []nova.ServerNetworks{}, nil
}

// ResolveNetwork is part of the Networking interface.
func (n *LegacyNovaNetworking) ResolveNetwork(name string, external bool) (string, error) {
	// Ignore external, it's a Neutron concept.
	if utils.IsValidUUIDString(name) {
		return name, nil
	}
	var networkIds []string
	networks, err := n.env.nova().ListNetworks()
	if err != nil {
		return "", err
	}
	for _, net := range networks {
		// Assuming a positive match on "" makes this behave the same as the
		// server-side filtering in Neutron networking.
		if name == "" || net.Label == name {
			networkIds = append(networkIds, net.Id)
		}
	}
	return processResolveNetworkIds(name, networkIds)
}

// Subnets is part of the Networking interface.
func (n *LegacyNovaNetworking) Subnets(
	instId instance.Id, subnetIds []corenetwork.Id,
) ([]corenetwork.SubnetInfo, error) {
	return nil, errors.NotSupportedf("nova subnet")
}

// NetworkInterfaces is part of the Networking interface.
func (n *LegacyNovaNetworking) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	return nil, errors.NotSupportedf("nova network interfaces")
}
