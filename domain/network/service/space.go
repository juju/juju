// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs/envcontext"
)

// Service provides the API for working with spaces.
type Service struct {
	// The space service needs the full state because we make use of the
	// UpsertSubnets method from the SubnetState.
	st     State
	logger logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// AddSpace creates and returns a new space.
func (s *Service) AddSpace(ctx context.Context, space network.SpaceInfo) (network.Id, error) {
	if !names.IsValidSpace(string(space.Name)) {
		return "", errors.NotValidf("space name %q", space.Name)
	}

	spaceID := space.ID
	if spaceID == "" {
		uuid, err := uuid.NewV7()
		if err != nil {
			return "", errors.Annotatef(err, "creating uuid for new space %q", space.Name)
		}
		spaceID = uuid.String()
	}

	subnetIDs := make([]string, len(space.Subnets))
	for i, subnet := range space.Subnets {
		subnetIDs[i] = subnet.ID.String()
	}
	if err := s.st.AddSpace(ctx, spaceID, string(space.Name), space.ProviderId, subnetIDs); err != nil {
		return "", errors.Trace(err)
	}
	return network.Id(spaceID), nil
}

// UpdateSpace updates the space name identified by the passed uuid.
func (s *Service) UpdateSpace(ctx context.Context, uuid string, name string) error {
	return errors.Trace(s.st.UpdateSpace(ctx, uuid, name))
}

// Space returns a space from state that matches the input ID.
// An error is returned if the space does not exist or if there was a problem
// accessing its information.
func (s *Service) Space(ctx context.Context, uuid string) (*network.SpaceInfo, error) {
	sp, err := s.st.GetSpace(ctx, uuid)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return sp, nil
}

// SpaceByName returns a space from state that matches the input name.
// An error is returned that satisfied errors.NotFound if the space was not found
// or an error static any problems fetching the given space.
func (s *Service) SpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error) {
	sp, err := s.st.GetSpaceByName(ctx, name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return sp, nil
}

// GetAllSpaces returns all spaces for the model.
func (s *Service) GetAllSpaces(ctx context.Context) (network.SpaceInfos, error) {
	spaces, err := s.st.GetAllSpaces(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return spaces, nil
}

// RemoveSpace deletes a space identified by its uuid.
func (s *Service) RemoveSpace(ctx context.Context, uuid string) error {
	return errors.Trace(s.st.DeleteSpace(ctx, uuid))
}

// ProviderService provides the API for working with network spaces.
type ProviderService struct {
	Service
	provider func(context.Context) (Provider, error)
}

// NewProviderService returns a new service reference wrapping the input state.
func NewProviderService(
	st State,
	provider providertracker.ProviderGetter[Provider],
	logger logger.Logger,
) *ProviderService {
	return &ProviderService{
		Service: Service{
			st:     st,
			logger: logger,
		},
		provider: provider,
	}
}

// ReloadSpaces loads spaces and subnets from the provider into state.
func (s *ProviderService) ReloadSpaces(ctx context.Context) error {
	callContext := envcontext.WithoutCredentialInvalidator(ctx)

	networkProvider, err := s.provider(ctx)
	if errors.Is(err, errors.NotSupported) {
		return errors.NotSupportedf("spaces discovery in a non-networking environ")
	}
	if err != nil {
		return errors.Trace(err)
	}

	// Retrieve the fan config from the model config.
	fanConfigStr, err := s.st.FanConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	fanConfig, err := network.ParseFanConfig(fanConfigStr)
	if err != nil {
		return errors.Trace(err)
	}

	canDiscoverSpaces, err := networkProvider.SupportsSpaceDiscovery(callContext)
	if err != nil {
		return errors.Trace(err)
	}

	if canDiscoverSpaces {
		spaces, err := networkProvider.Spaces(callContext)
		if err != nil {
			return errors.Trace(err)
		}

		s.Service.logger.Infof("discovered spaces: %s", spaces.String())

		providerSpaces := NewProviderSpaces(s, s.logger)
		if err := providerSpaces.saveSpaces(ctx, spaces, fanConfig); err != nil {
			return errors.Trace(err)
		}
		warnings, err := providerSpaces.deleteSpaces(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		for _, warning := range warnings {
			s.Service.logger.Tracef(warning)
		}
		return nil
	}

	s.Service.logger.Debugf("environ does not support space discovery, falling back to subnet discovery")
	subnets, err := networkProvider.Subnets(callContext, instance.UnknownId, nil)
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(nvinuesa): Here, the alpha space is scaffolding, it should be
	// replaced with the model's default space.
	return errors.Trace(s.saveProviderSubnets(ctx, subnets, network.AlphaSpaceId, fanConfig))
}

// SaveProviderSubnets loads subnets into state.
// Currently it does not delete removed subnets.
func (s *ProviderService) saveProviderSubnets(
	ctx context.Context,
	subnets []network.SubnetInfo,
	spaceUUID string,
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
		subnetToUpsert.SpaceID = spaceUUID
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

			// Add the fan subnet to the upsert list.
			fanSubnetToUpsert := subnet
			fanSubnetToUpsert.ProviderId = network.Id(fanSubnetID)
			fanSubnetToUpsert.SetFan(fanSubnetToUpsert.CIDR, fan.Overlay.String())
			fanSubnetToUpsert.SpaceID = spaceUUID

			fanInfo := &network.FanCIDRs{
				FanLocalUnderlay: fanSubnetToUpsert.CIDR,
				FanOverlay:       fan.Overlay.String(),
			}
			fanSubnetToUpsert.FanInfo = fanInfo
			fanSubnetToUpsert.CIDR = overlaySegment.String()

			subnetsToUpsert = append(subnetsToUpsert, fanSubnetToUpsert)
		}
	}

	if len(subnetsToUpsert) > 0 {
		return errors.Trace(s.upsertProviderSubnets(ctx, subnetsToUpsert))
	}

	return nil
}

// upsertProviderSubnets shims the state method for upserting subnets, and also
// makes sure a uuid is inserted by checking if one was provided otherwise
// create a new UUID v7.
func (s *ProviderService) upsertProviderSubnets(ctx context.Context, subnetsToUpsert network.SubnetInfos) error {
	for i, sn := range subnetsToUpsert {
		if sn.ID.String() == "" {
			uuid, err := uuid.NewV7()
			if err != nil {
				return errors.Trace(err)
			}
			subnetsToUpsert[i].ID = network.Id(uuid.String())
		}

	}
	if err := s.st.UpsertSubnets(ctx, subnetsToUpsert); err != nil && !errors.Is(err, errors.AlreadyExists) {
		return errors.Trace(err)
	}
	return nil
}

// generateFanSubnetID generates a correct ID for a subnet of type fan overlay.
func generateFanSubnetID(subnetNetwork, providerID string) string {
	subnetWithDashes := strings.Replace(strings.Replace(subnetNetwork, ".", "-", -1), "/", "-", -1)
	return fmt.Sprintf("%s-%s-%s", providerID, network.InFan, subnetWithDashes)
}

// ProviderSpaces defines a set of operations to perform when dealing with
// provider spaces. SaveSpaces, DeleteSpaces are operations for setting state
// in the persistence layer.
type ProviderSpaces struct {
	modelSpaceMap map[network.Id]network.SpaceInfo
	updatedSpaces network.IDSet
	spaceService  *ProviderService
	logger        logger.Logger
}

// NewProviderSpaces creates a new ProviderSpaces to perform a series of
// operations.
func NewProviderSpaces(spaceService *ProviderService, logger logger.Logger) *ProviderSpaces {
	return &ProviderSpaces{
		spaceService: spaceService,
		logger:       logger,

		modelSpaceMap: make(map[network.Id]network.SpaceInfo),
		updatedSpaces: network.MakeIDSet(),
	}
}

// SaveSpaces consumes provider spaces and saves the spaces as subnets on a
// provider.
func (s *ProviderSpaces) saveSpaces(ctx context.Context, providerSpaces []network.SpaceInfo, fanConfig network.FanConfig) error {
	stateSpaces, err := s.spaceService.GetAllSpaces(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	spaceNames := set.NewStrings()
	for _, space := range stateSpaces {
		s.modelSpaceMap[space.ProviderId] = space
		spaceNames.Add(string(space.Name))
	}

	for _, spaceInfo := range providerSpaces {
		// Check if the space is already in state,
		// in which case we know its name.
		var spaceID string
		stateSpace, ok := s.modelSpaceMap[spaceInfo.ProviderId]
		if ok {
			spaceID = stateSpace.ID
		} else {
			// The space is new, we need to create a valid name for it in state.
			// Convert the name into a valid name that is not already in use.
			spaceName := network.ConvertSpaceName(string(spaceInfo.Name), spaceNames)

			s.logger.Debugf("Adding space %s from provider %s", spaceName, string(spaceInfo.ProviderId))
			spaceUUID, err := s.spaceService.AddSpace(
				ctx,
				network.SpaceInfo{
					Name:       network.SpaceName(spaceName),
					ProviderId: spaceInfo.ProviderId,
				},
			)
			if err != nil {
				return errors.Trace(err)
			}

			spaceNames.Add(spaceName)

			// To ensure that we can remove spaces, we back-fill the new spaces
			// onto the modelSpaceMap.
			s.modelSpaceMap[spaceInfo.ProviderId] = network.SpaceInfo{
				ID:         spaceUUID.String(),
				Name:       network.SpaceName(spaceName),
				ProviderId: spaceInfo.ProviderId,
			}
			spaceID = spaceUUID.String()
		}

		err = s.spaceService.saveProviderSubnets(ctx, spaceInfo.Subnets, spaceID, fanConfig)
		if err != nil {
			return errors.Trace(err)
		}

		s.updatedSpaces.Add(spaceInfo.ProviderId)
	}

	return nil
}

// DeltaSpaces returns all the spaces that haven't been updated.
func (s *ProviderSpaces) deltaSpaces() network.IDSet {
	// Workout the difference between all the current spaces vs what was
	// actually changed.
	allStateSpaces := network.MakeIDSet()
	for providerID := range s.modelSpaceMap {
		allStateSpaces.Add(providerID)
	}

	return allStateSpaces.Difference(s.updatedSpaces)
}

// DeleteSpaces will attempt to delete any unused spaces after a SaveSpaces has
// been called.
// If there are no spaces to be deleted, it will exit out early.
func (s *ProviderSpaces) deleteSpaces(ctx context.Context) ([]string, error) {
	// Exit early if there is nothing to do.
	if len(s.modelSpaceMap) == 0 {
		return nil, nil
	}

	// Then check if the delta spaces are empty, if it's also empty, exit again.
	// We do it after modelSpaceMap as we create a types to create this, which
	// seems pretty wasteful.
	remnantSpaces := s.deltaSpaces()
	if len(remnantSpaces) == 0 {
		return nil, nil
	}

	// TODO (manadart 2024-01-29): The alpha space ID here is scaffolding and
	// should be replaced with the configured model default space upon
	// migrating this logic to Dqlite.
	defaultEndpointBinding := network.AlphaSpaceId

	var warnings []string
	for _, providerID := range remnantSpaces.SortedValues() {
		// If the space is not in state or the name is not in space names, then
		// we can ignore it.
		space, ok := s.modelSpaceMap[providerID]
		if !ok {
			// No warning here, the space was just not found.
			continue
		} else if space.Name == network.AlphaSpaceName ||
			space.ID == defaultEndpointBinding {

			warning := fmt.Sprintf("Unable to delete space %q. Space is used as the default space.", space.Name)
			warnings = append(warnings, warning)
			continue
		}

		// TODO(nvinuesa): This check is removed. We are going to handle
		// this validation by referential integrity (between spaces and
		// endpoint bindings).
		// Check all endpoint bindings found within a model. If they reference
		// a space name, then ignore then space for removal.

		// TODO(nvinuesa): This check is removed. We are going to handle
		// this validation by referential integrity (between spaces and
		// constraints).
		// Check to see if any space is within any constraints, if they are,
		// ignore them for now.

		if err := s.spaceService.RemoveSpace(ctx, space.ID); err != nil {
			return warnings, errors.Trace(err)
		}
	}

	return warnings, nil
}
