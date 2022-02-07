// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/names/v4"
)

const deployerFacade = "Deployer"

// State provides access to the deployer worker's idea of the state.
type State struct {
	facade base.FacadeCaller
}

// NewState creates a new State instance that makes API calls
// through the given caller.
func NewState(caller base.APICaller) *State {
	facadeCaller := base.NewFacadeCaller(caller, deployerFacade)
	return &State{facade: facadeCaller}

}

// unitLife returns the lifecycle state of the given unit.
func (st *State) unitLife(tag names.UnitTag) (life.Value, error) {
	return common.OneLife(st.facade, tag)
}

// Unit returns the unit with the given tag.
func (st *State) Unit(tag names.UnitTag) (*Unit, error) {
	life, err := st.unitLife(tag)
	if err != nil {
		return nil, err
	}
	return &Unit{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// Machine returns the machine with the given tag.
func (st *State) Machine(tag names.MachineTag) (*Machine, error) {
	// TODO(dfc) this cannot return an error any more
	return &Machine{
		tag: tag,
		st:  st,
	}, nil
}

// ConnectionInfo returns all the address information that the deployer task
// needs in one call.
func (st *State) ConnectionInfo() (result params.DeployerConnectionValues, err error) {
	err = st.facade.FacadeCall("ConnectionInfo", nil, &result)
	return result, err
}
