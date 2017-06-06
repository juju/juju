// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

func (st *State) getModelSubnets() (set.Strings, error) {
	subnets, err := st.AllSubnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelSubnetIds := make(set.Strings)
	for _, subnet := range subnets {
		modelSubnetIds.Add(string(subnet.ProviderId()))
	}
	return modelSubnetIds, nil
}

// ReloadSpaces loads spaces and subnets from provider specified by environ into state.
// Currently it's an append-only operation, no spaces/subnets are deleted.
func (st *State) ReloadSpaces(environ environs.Environ) error {
	netEnviron, ok := environs.SupportsNetworking(environ)
	if !ok {
		return errors.NotSupportedf("spaces discovery in a non-networking environ")
	}
	canDiscoverSpaces, err := netEnviron.SupportsSpaceDiscovery()
	if err != nil {
		return errors.Trace(err)
	}
	if canDiscoverSpaces {
		spaces, err := netEnviron.Spaces()
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(st.SaveSpacesFromProvider(spaces))
	} else {
		logger.Debugf("environ does not support space discovery, falling back to subnet discovery")
		subnets, err := netEnviron.Subnets(instance.UnknownId, nil)
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(st.SaveSubnetsFromProvider(subnets))
	}
}

// SaveSubnetsFromProvider loads subnets into state.
// Currently it does not delete removed subnets.
func (st *State) SaveSubnetsFromProvider(subnets []network.SubnetInfo) error {
	modelSubnetIds, err := st.getModelSubnets()
	if err != nil {
		return errors.Trace(err)
	}
	for _, subnet := range subnets {
		if modelSubnetIds.Contains(string(subnet.ProviderId)) {
			continue
		}
		var firstZone string
		if len(subnet.AvailabilityZones) > 0 {
			firstZone = subnet.AvailabilityZones[0]
		}
		_, err := st.AddSubnet(SubnetInfo{
			ProviderId:        subnet.ProviderId,
			ProviderNetworkId: subnet.ProviderNetworkId,
			CIDR:              subnet.CIDR,
			VLANTag:           subnet.VLANTag,
			AvailabilityZone:  firstZone,
		})
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// SaveSpacesFromProvider loads providerSpaces into state.
// Currently it does not delete removed spaces.
func (st *State) SaveSpacesFromProvider(providerSpaces []network.SpaceInfo) error {
	stateSpaces, err := st.AllSpaces()
	if err != nil {
		return errors.Trace(err)
	}
	modelSpaceMap := make(map[network.Id]*Space)
	spaceNames := make(set.Strings)
	for _, space := range stateSpaces {
		modelSpaceMap[space.ProviderId()] = space
		spaceNames.Add(space.Name())
	}

	modelSubnetIds, err := st.getModelSubnets()
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(mfoord): we need to delete spaces and subnets that no longer
	// exist, so long as they're not in use.
	for _, space := range providerSpaces {
		// Check if the space is already in state, in which case we know
		// its name.
		stateSpace, ok := modelSpaceMap[space.ProviderId]
		var spaceTag names.SpaceTag
		if ok {
			spaceName := stateSpace.Name()
			if !names.IsValidSpace(spaceName) {
				// Can only happen if an invalid name is stored
				// in state.
				logger.Errorf("space %q has an invalid name, ignoring", spaceName)
				continue

			}
			spaceTag = names.NewSpaceTag(spaceName)

		} else {
			// The space is new, we need to create a valid name for it
			// in state.
			spaceName := string(space.Name)
			// Convert the name into a valid name that isn't already in
			// use.
			spaceName = network.ConvertSpaceName(spaceName, spaceNames)
			spaceNames.Add(spaceName)
			spaceTag = names.NewSpaceTag(spaceName)
			// We need to create the space.

			logger.Debugf("Adding space %s from provider %s", spaceTag.String(), string(space.ProviderId))
			_, err = st.AddSpace(spaceTag.Id(), space.ProviderId, []string{}, false)
			if err != nil {
				return errors.Trace(err)
			}
		}

		for _, subnet := range space.Subnets {
			if modelSubnetIds.Contains(string(subnet.ProviderId)) {
				continue
			}
			var firstZone string
			if len(subnet.AvailabilityZones) > 0 {
				firstZone = subnet.AvailabilityZones[0]
			}
			_, err = st.AddSubnet(SubnetInfo{
				ProviderId:        subnet.ProviderId,
				ProviderNetworkId: subnet.ProviderNetworkId,
				CIDR:              subnet.CIDR,
				SpaceName:         spaceTag.Id(),
				VLANTag:           subnet.VLANTag,
				AvailabilityZone:  firstZone,
			})
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}
