// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	domainnetwork "github.com/juju/juju/domain/network"
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

// GetSpace returns a space from state that matches the input ID. If the space
// is not found, an error is returned matching
// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
func (s *Service) GetSpace(ctx context.Context, uuid network.SpaceUUID) (*network.SpaceInfo, error) {
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

// RemoveSpace removes a space identified by the given name. It can handle forced removal and supports dry-run mode.
func (s *Service) RemoveSpace(ctx context.Context, name network.SpaceName, force bool,
	dryRun bool) (domainnetwork.RemoveSpaceViolations, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	violations, err := s.st.RemoveSpace(ctx, name, force, dryRun)
	return violations, errors.Capture(err)
}
