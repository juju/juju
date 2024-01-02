// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/network"
)

// State describes retrieval and persistence methods needed for the network
// domain service.
type State interface {
	// AddSpace creates and returns a new space.
	AddSpace(ctx context.Context, uuid network.Id, name string, providerID network.Id, subnetIDs []string) error
	// GetSpace returns the space by UUID.
	GetSpace(ctx context.Context, uuid string) (*network.SpaceInfo, error)
	// GetSpaceByName returns the space by name.
	GetSpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// UpdateSpace updates the space identified by the passed uuid.
	UpdateSpace(ctx context.Context, uuid string, name string) error
	// DeleteSpace deletes the space identified by the passed uuid.
	DeleteSpace(ctx context.Context, uuid string) error

	// Subnet (sub-domain) methods

	// UpdateSubnet updates the subnet identified by the passed uuid.
	UpdateSubnet(ctx context.Context, uuid string, spaceID string) error
	// AddSubnet creates a subnet.
	AddSubnet(ctx context.Context, uuid utils.UUID, cidr string, providerID network.Id, providerNetworkID network.Id, VLANTag int, availabilityZones []string, spaceUUID string, fanInfo *network.FanCIDRs) error
	// UpsertSubnets updates or adds each one of the provided subnets in one
	// transaction.
	UpsertSubnets(ctx context.Context, subnets []network.SubnetInfo) error
}

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// SpaceService provides the API for working with spaces.
type SpaceService struct {
	st     State
	logger Logger
}

// NewSpaceService returns a new service reference wrapping the input state.
func NewSpaceService(st State, logger Logger) *SpaceService {
	return &SpaceService{
		st:     st,
		logger: logger,
	}
}

// AddSpace creates and returns a new space.
func (s *SpaceService) AddSpace(ctx context.Context, name string, providerID network.Id, subnetIDs []string) (network.Id, error) {
	if !names.IsValidSpace(name) {
		return "", errors.NotValidf("space name %q", name)
	}

	uuid, err := utils.NewUUID()
	if err != nil {
		return "", errors.Annotatef(err, "creating uuid for new space %q", name)
	}
	spaceID := network.Id(uuid.String())

	if err := s.st.AddSpace(ctx, spaceID, name, providerID, subnetIDs); err != nil {
		return "", errors.Trace(err)
	}
	return spaceID, nil
}

// Space returns a space from state that matches the input ID.
// An error is returned if the space does not exist or if there was a problem
// accessing its information.
func (s *SpaceService) Space(ctx context.Context, uuid string) (*network.SpaceInfo, error) {
	return s.st.GetSpace(ctx, uuid)
}

// SpaceByName returns a space from state that matches the input name.
// An error is returned that satisfied errors.NotFound if the space was not found
// or an error static any problems fetching the given space.
func (s *SpaceService) SpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error) {
	return s.st.GetSpaceByName(ctx, name)
}

// GetAllSpaces returns all spaces for the model.
func (s *SpaceService) GetAllSpaces(ctx context.Context) (network.SpaceInfos, error) {
	return s.st.GetAllSpaces(ctx)
}

// Remove deletes a space identified by its uuid.
func (s *SpaceService) Remove(ctx context.Context, uuid string) error {
	return s.st.DeleteSpace(ctx, uuid)
}

// SaveProviderSubnets loads subnets into state.
// Currently it does not delete removed subnets.
func (s *SpaceService) SaveProviderSubnets(
	ctx context.Context,
	subnets []network.SubnetInfo,
	spaceUUID network.Id,
	fans network.FanConfig,
) error {

	var subnetsToUpsert []network.SubnetInfo

	for _, subnet := range subnets {
		ip, _, err := net.ParseCIDR(subnet.CIDR)
		if err != nil {
			return errors.Trace(err)
		}
		if ip.IsInterfaceLocalMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
			continue
		}

		// Add the subnet with the provided space UUID to the upsert list.
		subnetToUpsert := subnet
		subnetToUpsert.SpaceID = spaceUUID.String()
		subnetsToUpsert = append(subnetsToUpsert, subnetToUpsert)

		// Iterate over fan configs.
		for _, fan := range fans {
			_, subnetNet, err := net.ParseCIDR(subnet.CIDR)
			if err != nil {
				return errors.Trace(err)
			}
			if subnetNet.IP.To4() == nil {
				s.logger.Debugf("%s address is not an IPv4 address", subnetNet.IP)
				continue
			}
			// Compute the overlay segment.
			overlaySegment, err := network.CalculateOverlaySegment(subnet.CIDR, fan)
			if err != nil {
				return errors.Trace(err)
			} else if overlaySegment == nil {
				// network.CalculateOverlaySegment can return
				// (nil, nil) so we need to make sure not to do
				// anything when overlaySegment is nil.
				continue
			}
			fanSubnetID := generateFanSubnetID(subnetNet.String(), subnet.ProviderId.String())
			if overlaySegment != nil {
				// Add the fan subnet to the upsert list.
				fanSubnetToUpsert := subnet
				fanSubnetToUpsert.ProviderId = network.Id(fanSubnetID)
				fanSubnetToUpsert.SetFan(fanSubnetToUpsert.CIDR, fan.Overlay.String())
				fanSubnetToUpsert.SpaceID = spaceUUID.String()

				fanInfo := &network.FanCIDRs{
					FanLocalUnderlay: fanSubnetToUpsert.CIDR,
					FanOverlay:       fan.Overlay.String(),
				}
				fanSubnetToUpsert.FanInfo = fanInfo
				fanSubnetToUpsert.CIDR = overlaySegment.String()

				subnetsToUpsert = append(subnetsToUpsert, fanSubnetToUpsert)
			}
		}
	}

	if len(subnetsToUpsert) > 0 {
		if err := s.st.UpsertSubnets(ctx, subnetsToUpsert); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// generateFanSubnetID generates a correct ID for a subnet of type fan overlay.
func generateFanSubnetID(subnetNetwork, providerID string) string {
	subnetWithDashes := strings.Replace(strings.Replace(subnetNetwork, ".", "-", -1), "/", "-", -1)
	return fmt.Sprintf("%s-%s-%s", providerID, network.InFan, subnetWithDashes)
}
