// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.resource.state")

// RawState defines the functionality needed from state.State for resources.
type RawState interface {
	rawSpecState
}

// State exposes the state functionality needed for resources.
type State struct {
	*specState
}

// NewState returns a new State for the given raw Juju state.
func NewState(raw RawState) *State {
	return &State{
		specState: &specState{raw},
	}
}
