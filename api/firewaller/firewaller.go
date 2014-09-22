// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

const firewallerFacade = "Firewaller"

// State provides access to the Firewaller API facade.
type State struct {
	facade base.FacadeCaller
	*common.EnvironWatcher
}

// newStateV0 creates a new client-side Firewaller facade, version 0.
func newStateV0(caller base.APICaller) *State {
	facadeCaller := base.NewFacadeCallerForVersion(caller, firewallerFacade, 0)
	return &State{
		facade:         facadeCaller,
		EnvironWatcher: common.NewEnvironWatcher(facadeCaller),
	}
}

// newStateV0 creates a new client-side Firewaller facade, version 1.
func newStateV1(caller base.APICaller) *State {
	facadeCaller := base.NewFacadeCallerForVersion(caller, firewallerFacade, 1)
	return &State{
		facade:         facadeCaller,
		EnvironWatcher: common.NewEnvironWatcher(facadeCaller),
	}
}

// newStateBestVersion creates a new client-side Firewaller facade
// with the best API version supported by both the client and the
// server.
//
// TODO(dimitern) Once the firewaller worker uses V1, make this
// the default constructor.
func newStateBestVersion(caller base.APICaller) *State {
	facadeCaller := base.NewFacadeCaller(caller, firewallerFacade)
	return &State{
		facade:         facadeCaller,
		EnvironWatcher: common.NewEnvironWatcher(facadeCaller),
	}
}

// NewState creates a new client-side Firewaller facade.
// Defined like this to allow patching during tests.
var NewState = newStateV0

// BestAPIVersion returns the API version that we were able to
// determine is supported by both the client and the API Server.
func (st *State) BestAPIVersion() int {
	return st.facade.BestAPIVersion()
}

// life requests the life cycle of the given entity from the server.
func (st *State) life(tag names.Tag) (params.Life, error) {
	return common.Life(st.facade, tag)
}

// Unit provides access to methods of a state.Unit through the facade.
func (st *State) Unit(tag names.UnitTag) (*Unit, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Unit{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// Machine provides access to methods of a state.Machine through the
// facade.
func (st *State) Machine(tag names.MachineTag) (*Machine, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Machine{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// WatchEnvironMachines returns a StringsWatcher that notifies of
// changes to the life cycles of the top level machines in the current
// environment.
func (st *State) WatchEnvironMachines() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := st.facade.FacadeCall("WatchEnvironMachines", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchOpenedPorts returns a StringsWatcher that notifies of
// changes to the opened ports for the given environment tag.
func (st *State) WatchOpenedPorts(tag names.EnvironTag) (watcher.StringsWatcher, error) {
	if st.BestAPIVersion() < 1 {
		// WatchOpenedPorts() was introduced in FirewallerAPIV1.
		return nil, errors.NotImplementedf("WatchOpenedPorts() (need V1+)")
	}
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	err := st.facade.FacadeCall("WatchOpenedPorts", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}
