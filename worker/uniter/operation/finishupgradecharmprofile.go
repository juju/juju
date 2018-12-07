// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"
)

type finishUpgradeCharmProfile struct {
	DoesNotRequireMachineLock

	kind      Kind
	charmURL  *charm.URL
	callbacks Callbacks
}

// String is part of the Operation interface.
func (d *finishUpgradeCharmProfile) String() string {
	return fmt.Sprintf("finish upgrade charm profile")
}

// Prepare is part of the Operation interface.
func (d *finishUpgradeCharmProfile) Prepare(state State) (*State, error) {
	return d.getState(state, Pending), nil
}

// Execute is part of the Operation interface.
func (d *finishUpgradeCharmProfile) Execute(state State) (*State, error) {
	// Ensure that we always clean up the LXD profile status.
	if err := d.callbacks.RemoveUpgradeCharmProfileData(); err != nil {
		return nil, errors.Trace(err)
	}
	return d.getState(state, Done), nil
}

// Commit is part of the Operation interface.
func (d *finishUpgradeCharmProfile) Commit(state State) (*State, error) {
	// make no change to state
	return &state, nil
}

func (d *finishUpgradeCharmProfile) getState(state State, step Step) *State {
	return stateChange{
		Kind:     d.kind,
		Step:     step,
		CharmURL: d.charmURL,
	}.apply(state)
}
