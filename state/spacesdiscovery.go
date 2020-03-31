// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	corenetwork "github.com/juju/juju/core/network"
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

// SaveProviderSubnets loads subnets into state.
// Currently it does not delete removed subnets.
func (st *State) SaveProviderSubnets(subnets []corenetwork.SubnetInfo, spaceID string) error {
	modelSubnetIds, err := st.getModelSubnets()
	if err != nil {
		return errors.Trace(err)
	}

	for _, subnet := range subnets {
		ip, _, err := net.ParseCIDR(subnet.CIDR)
		if err != nil {
			return errors.Trace(err)
		}
		if ip.IsInterfaceLocalMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
			continue
		}

		subnet.SpaceID = spaceID
		if modelSubnetIds.Contains(string(subnet.ProviderId)) {
			err = st.SubnetUpdate(subnet)
		} else {
			_, err = st.AddSubnet(subnet)
		}

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
			id := fmt.Sprintf("%s-%s-%s", subnet.ProviderId, corenetwork.InFan, subnetWithDashes)
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
				subnet.SpaceID = spaceID
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

// SaveProviderSpaces loads providerSpaces into state.
// Currently it does not delete removed spaces.
func (st *State) SaveProviderSpaces(providerSpaces []corenetwork.SpaceInfo) error {
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
	for _, spaceInfo := range providerSpaces {
		// Check if the space is already in state,
		// in which case we know its name.
		stateSpace, ok := modelSpaceMap[spaceInfo.ProviderId]
		var spaceId string
		if ok {
			spaceId = stateSpace.Id()
		} else {
			// The space is new, we need to create a valid name for it in state.
			// Convert the name into a valid name that is not already in use.
			spaceName := corenetwork.ConvertSpaceName(string(spaceInfo.Name), spaceNames)

			logger.Debugf("Adding space %s from provider %s", spaceName, string(spaceInfo.ProviderId))
			space, err := st.AddSpace(spaceName, spaceInfo.ProviderId, []string{}, false)
			if err != nil {
				return errors.Trace(err)
			}

			spaceNames.Add(spaceName)
			spaceId = space.Id()
		}

		err = st.SaveProviderSubnets(spaceInfo.Subnets, spaceId)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
