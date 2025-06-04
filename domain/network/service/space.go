// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/juju/collections/set"
	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/trace"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/errors"
)

// AddSpace creates and returns a new space.
func (s *Service) AddSpace(ctx context.Context, space network.SpaceInfo) (network.SpaceUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !names.IsValidSpace(string(space.Name)) {
		return "", errors.Errorf("space name %q not valid", space.Name).Add(networkerrors.SpaceNameNotValid)
	}

	spaceID := space.ID
	if spaceID == "" {
		var err error
		spaceID, err = network.NewSpaceUUID()
		if err != nil {
			return "", errors.Errorf("creating uuid for new space %q: %w", space.Name, err)
		}
	}

	subnetIDs := make([]string, len(space.Subnets))
	for i, subnet := range space.Subnets {
		subnetIDs[i] = subnet.ID.String()
	}
	if err := s.st.AddSpace(ctx, spaceID, space.Name, space.ProviderId, subnetIDs); err != nil {
		return "", errors.Capture(err)
	}
	return spaceID, nil
}

// UpdateSpace updates the space name identified by the passed uuid. If the
// space is not found, an error is returned matching
// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
func (s *Service) UpdateSpace(ctx context.Context, uuid network.SpaceUUID, name network.SpaceName) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return errors.Capture(s.st.UpdateSpace(ctx, uuid, name))
}

// Space returns a space from state that matches the input ID. If the space is
// not found, an error is returned matching
// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
func (s *Service) Space(ctx context.Context, uuid network.SpaceUUID) (*network.SpaceInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	sp, err := s.st.GetSpace(ctx, uuid)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return sp, nil
}

// SpaceByName returns a space from state that matches the input name. If the
// space is not found, an error is returned matching
// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
func (s *Service) SpaceByName(ctx context.Context, name network.SpaceName) (*network.SpaceInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	sp, err := s.st.GetSpaceByName(ctx, name)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return sp, nil
}

// GetAllSpaces returns all spaces for the model.
func (s *Service) GetAllSpaces(ctx context.Context) (network.SpaceInfos, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	spaces, err := s.st.GetAllSpaces(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return spaces, nil
}

// RemoveSpace deletes a space identified by its uuid. If the space is not
// found, an error is returned matching
// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
func (s *Service) RemoveSpace(ctx context.Context, uuid network.SpaceUUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return errors.Capture(s.st.DeleteSpace(ctx, uuid))
}

// ProviderService provides the API for working with network spaces.
type ProviderService struct {
	Service
	providerWithNetworking func(context.Context) (ProviderWithNetworking, error)
	providerWithZones      func(context.Context) (ProviderWithZones, error)
}

// NewProviderService returns a new service reference wrapping the input state.
func NewProviderService(
	st State,
	providerWithNetworking providertracker.ProviderGetter[ProviderWithNetworking],
	providerWithZones providertracker.ProviderGetter[ProviderWithZones],
	logger logger.Logger,
) *ProviderService {
	return &ProviderService{
		Service: Service{
			st:     st,
			logger: logger,
		},
		providerWithNetworking: providerWithNetworking,
		providerWithZones:      providerWithZones,
	}
}

// ReloadSpaces loads spaces and subnets from the provider into state.
func (s *ProviderService) ReloadSpaces(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	networkProvider, err := s.providerWithNetworking(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return errors.Errorf("spaces discovery in a non-networking environ %w", coreerrors.NotSupported)
	}
	if err != nil {
		return errors.Capture(err)
	}

	canDiscoverSpaces, err := networkProvider.SupportsSpaceDiscovery()
	if err != nil {
		return errors.Capture(err)
	}

	if canDiscoverSpaces {
		spaces, err := networkProvider.Spaces(ctx)
		if err != nil {
			return errors.Capture(err)
		}

		s.Service.logger.Infof(ctx, "discovered spaces: %s", spaces.String())

		providerSpaces := NewProviderSpaces(s, s.logger)
		if err := providerSpaces.saveSpaces(ctx, spaces); err != nil {
			return errors.Capture(err)
		}
		warnings, err := providerSpaces.deleteSpaces(ctx)
		if err != nil {
			return errors.Capture(err)
		}
		for _, warning := range warnings {
			s.Service.logger.Tracef(ctx, warning)
		}
		return nil
	}

	s.Service.logger.Debugf(ctx, "environ does not support space discovery, falling back to subnet discovery")
	subnets, err := networkProvider.Subnets(ctx, nil)
	if err != nil {
		return errors.Capture(err)
	}
	// TODO(nvinuesa): Here, the alpha space is scaffolding, it should be
	// replaced with the model's default space.
	return errors.Capture(s.saveProviderSubnets(ctx, subnets, network.AlphaSpaceId))
}

// SaveProviderSubnets loads subnets into state.
// Currently it does not delete removed subnets.
func (s *ProviderService) saveProviderSubnets(
	ctx context.Context,
	subnets []network.SubnetInfo,
	spaceUUID network.SpaceUUID,
) error {
	var subnetsToUpsert []network.SubnetInfo
	for _, subnet := range subnets {
		ip, _, err := net.ParseCIDR(subnet.CIDR)
		if err != nil {
			return errors.Capture(err)
		}
		if ip.IsInterfaceLocalMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
			continue
		}

		// Add the subnet with the provided space UUID to the upsert list.
		subnetToUpsert := subnet
		subnetToUpsert.SpaceID = spaceUUID
		subnetsToUpsert = append(subnetsToUpsert, subnetToUpsert)
	}

	if len(subnetsToUpsert) > 0 {
		return errors.Capture(s.upsertProviderSubnets(ctx, subnetsToUpsert))
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
				return errors.Capture(err)
			}
			subnetsToUpsert[i].ID = network.Id(uuid.String())
		}

	}
	if err := s.st.UpsertSubnets(ctx, subnetsToUpsert); err != nil && !errors.Is(err, coreerrors.AlreadyExists) {
		return errors.Capture(err)
	}
	return nil
}

// SupportsSpaces returns whether the provider supports spaces.
func (s *ProviderService) SupportsSpaces(ctx context.Context) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	networkProvider, err := s.providerWithNetworking(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return networkProvider.SupportsSpaces()
}

// SupportsSpaceDiscovery returns whether the provider supports discovering
// spaces from the provider.
func (s *ProviderService) SupportsSpaceDiscovery(ctx context.Context) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	networkProvider, err := s.providerWithNetworking(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return networkProvider.SupportsSpaceDiscovery()
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
func (s *ProviderSpaces) saveSpaces(ctx context.Context, providerSpaces []network.SpaceInfo) error {
	stateSpaces, err := s.spaceService.GetAllSpaces(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	spaceNames := set.NewStrings()
	for _, space := range stateSpaces {
		s.modelSpaceMap[space.ProviderId] = space
		spaceNames.Add(string(space.Name))
	}

	for _, spaceInfo := range providerSpaces {
		// Check if the space is already in state,
		// in which case we know its name.
		var spaceID network.SpaceUUID
		stateSpace, ok := s.modelSpaceMap[spaceInfo.ProviderId]
		if ok {
			spaceID = stateSpace.ID
		} else {
			// The space is new, we need to create a valid name for it in state.
			// Convert the name into a valid name that is not already in use.
			spaceName := network.ConvertSpaceName(string(spaceInfo.Name), spaceNames)

			s.logger.Debugf(ctx, "Adding space %s from providerWithNetworking %s", spaceName, string(spaceInfo.ProviderId))
			spaceUUID, err := s.spaceService.AddSpace(
				ctx,
				network.SpaceInfo{
					Name:       network.SpaceName(spaceName),
					ProviderId: spaceInfo.ProviderId,
				},
			)
			if err != nil {
				return errors.Capture(err)
			}

			spaceNames.Add(spaceName)

			// To ensure that we can remove spaces, we back-fill the new spaces
			// onto the modelSpaceMap.
			s.modelSpaceMap[spaceInfo.ProviderId] = network.SpaceInfo{
				ID:         spaceUUID,
				Name:       network.SpaceName(spaceName),
				ProviderId: spaceInfo.ProviderId,
			}
			spaceID = spaceUUID
		}

		err = s.spaceService.saveProviderSubnets(ctx, spaceInfo.Subnets, spaceID)
		if err != nil {
			return errors.Capture(err)
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

		// Check to see if any space is within any constraints, if they are,
		// ignore them for now.
		isUsedInConstraints, err := s.spaceService.st.IsSpaceUsedInConstraints(ctx, space.Name)
		if err != nil {
			return warnings, errors.Capture(err)
		} else if isUsedInConstraints {
			warning := fmt.Sprintf("Unable to delete space %q. Space is used in a constraint.", space.Name)
			warnings = append(warnings, warning)
			continue
		}

		if err := s.spaceService.RemoveSpace(ctx, space.ID); err != nil {
			return warnings, errors.Capture(err)
		}
	}

	return warnings, nil
}
