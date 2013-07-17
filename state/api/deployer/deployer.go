// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"

	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
)

// Deployer provides access to the Deployer API facade.
type Deployer struct {
	caller common.Caller
}

// New creates a new client-side Deployer facade.
func New(caller common.Caller) *Deployer {
	return &Deployer{caller}
}

// unitLife requests the lifecycle of the given unit from the server.
func (d *Deployer) unitLife(tag string) (params.Life, error) {
	var result params.LifeResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := d.caller.Call("Deployer", "", "Life", args, &result)
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

// Deployer provides access to methods of a state.Unit through the facade.
func (d *Deployer) Unit(tag string) (*Unit, error) {
	life, err := d.unitLife(tag)
	if err != nil {
		return nil, err
	}
	return &Unit{
		tag:    tag,
		life:   life,
		dstate: d,
	}, nil
}

// Machine provides access to methods of a state.Machine through the facade.
func (d *Deployer) Machine(tag string) (*Machine, error) {
	return &Machine{
		tag:    tag,
		dstate: d,
	}, nil
}
