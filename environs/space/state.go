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

func (s *spaceStateShim) AllSpaces() ([]Space, error) {
	spaces, err := s.State.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]Space, len(spaces))
	for i, space := range spaces {
		results[i] = space
	}
	return results, nil
}

func (s *spaceStateShim) AddSpace(name string, providerID network.Id, subnetIds []string, public bool) (Space, error) {
	result, err := s.State.AddSpace(name, providerID, subnetIds, public)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
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
