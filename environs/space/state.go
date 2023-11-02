// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

type spaceStateShim struct {
	*state.State
}

// NewState creates a space state shim.
func NewState(st *state.State) *spaceStateShim {
	return &spaceStateShim{st}
}

func (s *spaceStateShim) AllSpaces() ([]network.SpaceInfo, error) {
	spaces, err := s.State.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]network.SpaceInfo, len(spaces))
	for i, space := range spaces {
		spaceInfo, err := space.NetworkSpace()
		if err != nil {
			return nil, errors.Trace(err)
		}
		results[i] = spaceInfo
	}
	return results, nil
}

func (s *spaceStateShim) AddSpace(name string, providerID network.Id, subnetIds []string, public bool) (network.SpaceInfo, error) {
	result, err := s.State.AddSpace(name, providerID, subnetIds, public)
	if err != nil {
		return network.SpaceInfo{}, errors.Trace(err)
	}
	spaceInfo, err := result.NetworkSpace()
	if err != nil {
		return network.SpaceInfo{}, errors.Trace(err)
	}
	return spaceInfo, nil
}

// TODO(nvinuesa): This method is needed temporarily and must be removed when
// the new spaces domain has been finished and the mongodb state removed.
func (s *spaceStateShim) Life(spaceID string) (state.Life, error) {
	space, err := s.State.Space(spaceID)
	if err != nil {
		return state.Dead, errors.Trace(err)
	}
	return space.Life(), nil
}

// TODO(nvinuesa): This method is needed temporarily and must be removed when
// the new spaces domain has been finished and the mongodb state removed.
func (s *spaceStateShim) EnsureDead(spaceID string) error {
	space, err := s.State.Space(spaceID)
	if err != nil {
		return errors.Trace(err)
	}
	return space.EnsureDead()
}

// TODO(nvinuesa): This method is needed temporarily and must be removed when
// the new spaces domain has been finished and the mongodb state removed.
func (s *spaceStateShim) Remove(spaceID string) error {
	space, err := s.State.Space(spaceID)
	if err != nil {
		return errors.Trace(err)
	}
	return space.Remove()
}

func (s *spaceStateShim) ConstraintsBySpaceName(name string) ([]Constraints, error) {
	constraints, err := s.State.ConstraintsBySpaceName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]Constraints, len(constraints))
	for i, constraint := range constraints {
		results[i] = constraint
	}
	return results, nil
}
