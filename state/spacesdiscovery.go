// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
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
func (st *State) ReloadSpaces(environ environs.BootstrapEnviron) error {
	netEnviron, ok := environs.SupportsNetworking(environ)
	if !ok {
		return errors.NotSupportedf("spaces discovery in a non-networking environ")
	}

	ctx := CallContext(st)
	canDiscoverSpaces, err := netEnviron.SupportsSpaceDiscovery(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if canDiscoverSpaces {
		spaces, err := netEnviron.Spaces(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(st.SaveSpacesFromProvider(spaces))
	} else {
		logger.Debugf("environ does not support space discovery, falling back to subnet discovery")
		subnets, err := netEnviron.Subnets(ctx, instance.UnknownId, nil)
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(st.SaveSubnetsFromProvider(subnets, ""))
	}
}

// SaveSubnetsFromProvider loads subnets into state.
// Currently it does not delete removed subnets.
func (st *State) SaveSubnetsFromProvider(subnets []corenetwork.SubnetInfo, spaceName string) error {
	modelSubnetIds, err := st.getModelSubnets()
	if err != nil {
		return errors.Trace(err)
	}

	for _, subnet := range subnets {
		if modelSubnetIds.Contains(string(subnet.ProviderId)) {
			continue
		}
		ip, _, err := net.ParseCIDR(subnet.CIDR)
		if err != nil {
			return errors.Trace(err)
		}
		if ip.IsInterfaceLocalMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
			continue
		}

		subnet.SpaceName = spaceName
		_, err = st.AddSubnet(subnet)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// We process FAN subnets separately for clarity.
	m, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	cfg, err := m.ModelConfig()
	if err != nil {
		return errors.Trace(err)
	}
	fans, err := cfg.FanConfig()
	if err != nil {
		return errors.Trace(err)
	}
	if len(fans) == 0 {
		return nil
	}

	for _, subnet := range subnets {
		for _, fan := range fans {
			_, subnetNet, err := net.ParseCIDR(subnet.CIDR)
			if err != nil {
				return errors.Trace(err)
			}
			subnetWithDashes := strings.Replace(strings.Replace(subnetNet.String(), ".", "-", -1), "/", "-", -1)
			id := fmt.Sprintf("%s-INFAN-%s", subnet.ProviderId, subnetWithDashes)
			if modelSubnetIds.Contains(id) {
				continue
			}
			if subnetNet.IP.To4() == nil {
				logger.Debugf("%s address is not an IPv4 address.", subnetNet.IP)
				continue
			}
			overlaySegment, err := network.CalculateOverlaySegment(subnet.CIDR, fan)
			if err != nil {
				return errors.Trace(err)
			}
			if overlaySegment != nil {
				subnet.ProviderId = corenetwork.Id(id)
				subnet.SpaceName = spaceName
				subnet.SetFan(subnet.CIDR, fan.Overlay.String())
				subnet.CIDR = overlaySegment.String()

				_, err := st.AddSubnet(subnet)
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}

	return nil
}

// SaveSpacesFromProvider loads providerSpaces into state.
// Currently it does not delete removed spaces.
func (st *State) SaveSpacesFromProvider(providerSpaces []corenetwork.SpaceInfo) error {
	stateSpaces, err := st.AllSpaces()
	if err != nil {
		return errors.Trace(err)
	}
	modelSpaceMap := make(map[corenetwork.Id]*Space)
	spaceNames := make(set.Strings)
	for _, space := range stateSpaces {
		modelSpaceMap[space.ProviderId()] = space
		spaceNames.Add(space.Name())
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
			spaceName := space.Name
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

		err = st.SaveSubnetsFromProvider(space.Subnets, spaceTag.Id())
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
