// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

// The Juju-recognized states in which a payload might be.
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
		supported := okayStates.Values()
		sort.Strings(supported)
		states := strings.Join(supported, `", "`)
		msg := fmt.Sprintf(`status %q not supported; expected one of ["%s"]`, state, states)
		return errors.NewNotValid(nil, msg)
	}
	return nil
}
