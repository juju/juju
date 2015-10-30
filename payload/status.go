// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

// The Juju-recognized states in which a workload might be.
const (
	StateUndefined = ""
	//TODO(wwitzel3) remove defined throughout
	StateDefined  = "defined"
	StateStarting = "starting"
	StateRunning  = "running"
	StateStopping = "stopping"
	StateStopped  = "stopped"
)

var okayStates = set.NewStrings(
	StateStarting,
	StateRunning,
	StateStopping,
	StateStopped,
)

// ValidateState verifies the state passed in is a valid okayState.
func ValidateState(state string) error {
	if !okayStates.Contains(state) {
		return errors.NotValidf("state (%q)", state)
	}
	return nil
}
