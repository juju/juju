// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

// stateShim forwards and adapts state.State
// methods to Backing methods.
type stateShim struct {
}

// NewStateShim returns a new state shim.
func NewStateShim() (*stateShim, error) {
	return &stateShim{}, nil
}

func (s *stateShim) ConstraintsBySpaceName(spaceName string) ([]Constraints, error) {
	// TODO: This must be implemented using the domain services.
	return nil, nil
}

func (s *stateShim) IsController() bool {
	// TODO: This must be implemented using the domain services.
	return true
}
