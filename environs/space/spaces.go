// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
)

var logger = loggo.GetLogger("juju.environs.space")

// Constraints defines the methods supported by constraints used in the space context.
type Constraints interface{}

// ReloadSpacesState defines an in situ point of use type for ReloadSpaces
type ReloadSpacesState interface {
	// AllSpaces returns all spaces for the model.
	AllSpaces() ([]network.SpaceInfo, error)
	// AddSpace creates and returns a new space.
	AddSpace(string, network.Id, []string) (network.SpaceInfo, error)
	// SaveProviderSubnets loads subnets into state.
	SaveProviderSubnets([]network.SubnetInfo, string) error
	// ConstraintsBySpaceName returns all Constraints that include a positive
	// or negative space constraint for the input space name.
	ConstraintsBySpaceName(string) ([]Constraints, error)
	// AllEndpointBindingsSpaceNames returns a set of spaces names for all the
	// endpoint bindings.
	AllEndpointBindingsSpaceNames(network.SpaceInfos) (set.Strings, error)
	// Remove removes a Dead space. If the space is not Dead or it is already
	// removed, an error is returned.
	Remove(spaceID string) error
}

// ReloadSpaces loads spaces and subnets from provider specified by environ into state.
// Currently it's an append-only operation, no spaces/subnets are deleted.
func ReloadSpaces(ctx envcontext.ProviderCallContext, state ReloadSpacesState, environ environs.BootstrapEnviron) error {
	netEnviron, ok := environs.SupportsNetworking(environ)
	if !ok || netEnviron == nil {
		return errors.NotSupportedf("spaces discovery in a non-networking environ")
	}

	canDiscoverSpaces, err := netEnviron.SupportsSpaceDiscovery(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if canDiscoverSpaces {
		spaces, err := netEnviron.Spaces(ctx)
		if err != nil {
			return errors.Trace(err)
		}

		logger.Infof("discovered spaces: %s", spaces.String())

		providerSpaces := NewProviderSpaces(state)
		if err := providerSpaces.SaveSpaces(spaces); err != nil {
			return errors.Trace(err)
		}
		// TODO(nvinuesa): This is only temporary since DeleteSpaces
		// calls AllEndpointBindingsSpaceNames() which takes the
		// complete list of spaces as inputs. This will go away once
		// we finish migrating endpoint bindings to dqlite.
		allSpaces, err := state.AllSpaces()
		if err != nil {
			return errors.Trace(err)
		}
		warnings, err := providerSpaces.DeleteSpaces(allSpaces)
		if err != nil {
			return errors.Trace(err)
		}
		for _, warning := range warnings {
			logger.Tracef(warning)
		}
		return nil
	}

	logger.Debugf("environ does not support space discovery, falling back to subnet discovery")
	subnets, err := netEnviron.Subnets(ctx, instance.UnknownId, nil)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(state.SaveProviderSubnets(subnets, ""))
}

// ProviderSpaces defines a set of operations to perform when dealing with
// provider spaces. SaveSpaces, DeleteSpaces are operations for setting state
// in the persistence layer.
type ProviderSpaces struct {
	state         ReloadSpacesState
	modelSpaceMap map[network.Id]network.SpaceInfo
	updatedSpaces network.IDSet
}

// NewProviderSpaces creates a new ProviderSpaces to perform a series of
// operations.
func NewProviderSpaces(st ReloadSpacesState) *ProviderSpaces {
	return &ProviderSpaces{
		state: st,

		modelSpaceMap: make(map[network.Id]network.SpaceInfo),
		updatedSpaces: network.MakeIDSet(),
	}
}

// SaveSpaces consumes provider spaces and saves the spaces as subnets on a
// provider.
func (s *ProviderSpaces) SaveSpaces(providerSpaces []network.SpaceInfo) error {
	stateSpaces, err := s.state.AllSpaces()
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

			logger.Debugf("Adding space %s from provider %s", spaceName, string(spaceInfo.ProviderId))
			space, err := s.state.AddSpace(spaceName, spaceInfo.ProviderId, []string{})
			if err != nil {
				return errors.Trace(err)
			}

			spaceNames.Add(spaceName)
			spaceID = space.ID

			// To ensure that we can remove spaces, we back-fill the new spaces
			// onto the modelSpaceMap.
			s.modelSpaceMap[space.ProviderId] = space
		}

		err = s.state.SaveProviderSubnets(spaceInfo.Subnets, spaceID)
		if err != nil {
			return errors.Trace(err)
		}

		s.updatedSpaces.Add(spaceInfo.ProviderId)
	}

	return nil
}

// DeltaSpaces returns all the spaces that haven't been updated.
func (s *ProviderSpaces) DeltaSpaces() network.IDSet {
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
func (s *ProviderSpaces) DeleteSpaces(allSpaces network.SpaceInfos) ([]string, error) {
	// Exit early if there is nothing to do.
	if len(s.modelSpaceMap) == 0 {
		return nil, nil
	}

	// Then check if the delta spaces are empty, if it's also empty, exit again.
	// We do it after modelSpaceMap as we create a types to create this, which
	// seems pretty wasteful.
	remnantSpaces := s.DeltaSpaces()
	if len(remnantSpaces) == 0 {
		return nil, nil
	}

	// TODO (manadart 2024-01-29): The alpha space ID here is scaffolding and
	// should be replaced with the configured model default space upon
	// migrating this logic to Dqlite.
	defaultEndpointBinding := network.AlphaSpaceId

	allEndpointBindings, err := s.state.AllEndpointBindingsSpaceNames(allSpaces)
	if err != nil {
		return nil, errors.Trace(err)
	}

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

		// Check all endpoint bindings found within a model. If they reference
		// a space name, then ignore then space for removal.
		if allEndpointBindings.Contains(string(space.Name)) {
			warning := fmt.Sprintf("Unable to delete space %q. Space is used as a endpoint binding.", space.Name)
			warnings = append(warnings, warning)
			continue
		}

		// Check to see if any space is within any constraints, if they are,
		// ignore them for now.
		if constraints, err := s.state.ConstraintsBySpaceName(string(space.Name)); err != nil || len(constraints) > 0 {
			warning := fmt.Sprintf("Unable to delete space %q. Space is used in a constraint.", space.Name)
			warnings = append(warnings, warning)
			continue
		}

		if err := s.state.Remove(space.ID); err != nil {
			return warnings, errors.Trace(err)
		}
	}

	return warnings, nil
}
