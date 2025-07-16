// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

// stateShim forwards and adapts state.State
// methods to Backing methods.
type stateShim struct {
	*state.State
}

// NewStateShim returns a new state shim.
func NewStateShim(st *state.State) (*stateShim, error) {
	return &stateShim{
		State: st,
	}, nil
}

func (s *stateShim) ConstraintsBySpaceName(spaceName string) ([]Constraints, error) {
	found, err := s.State.ConstraintsBySpaceName(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cons := make([]Constraints, len(found))
	for i, v := range found {
		cons[i] = v
	}
	return cons, nil
}
