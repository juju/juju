// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	jujucontroller "github.com/juju/juju/controller"

	"github.com/juju/juju/domain"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory domain.DBFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// ControllerConfig ...
func (s *State) ControllerConfig(ctx context.Context) (jujucontroller.Config, error) {
	return nil, nil
}
