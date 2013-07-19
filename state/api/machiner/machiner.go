// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"fmt"

	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
)

// Machiner provides access to the Machiner API facade.
type Machiner struct {
	caller common.Caller
}

// New creates a new client-side Machiner facade.
func New(caller common.Caller) *Machiner {
	return &Machiner{caller}
}

// machineLife requests the lifecycle of the given machine from the server.
func (m *Machiner) machineLife(tag string) (params.Life, error) {
	var result params.LifeResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := m.caller.Call("Machiner", "", "Life", args, &result)
	if err != nil {
		return "", err
	}
	if len(result.Results) != 1 {
		return "", fmt.Errorf("expected one result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return "", err
	}
	return result.Results[0].Life, nil
}

// Machine provides access to methods of a state.Machine through the facade.
func (m *Machiner) Machine(tag string) (*Machine, error) {
	life, err := m.machineLife(tag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		tag:    tag,
		life:   life,
		mstate: m,
	}, nil
}
