// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"

	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
)

// DeployerAPI provides access to the Deployer API facade.
type DeployerAPI struct {
	*common.Remover
	*common.PasswordChanger
	*common.LifeGetter
	*common.Addresser

	st         *state.State
	resources  *common.Resources
	authorizer common.Authorizer
	cache	map[string]interface{}
}

// getAllUnits returns a list of all principal and subordinate units
// assigned to the given machine.
func getAllUnits(st *state.State, machineTag string) ([]string, error) {
	_, id, err := names.ParseTag(machineTag, names.MachineTagKind)
	if err != nil {
		return nil, err
	}
	machine, err := st.Machine(id)
	if err != nil {
		return nil, err
	}
	// Start a watcher on machine's units, read the initial event and stop it.
	watch := machine.WatchUnits()
	defer watch.Stop()
	if units, ok := <-watch.Changes(); ok {
		return units, nil
	}
	return nil, fmt.Errorf("cannot obtain units of machine %q: %v", machineTag, watch.Err())
}

// NewDeployerAPI creates a new server-side DeployerAPI facade.
func NewDeployerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*DeployerAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	getAuthFunc := func() (common.AuthFunc, error) {
		// Get all units of the machine and cache them.
		thisMachineTag := authorizer.GetAuthTag()
		units, err := getAllUnits(st, thisMachineTag)
		if err != nil {
			return nil, err
		}
		// Then we just check if the unit is already known.
		return func(tag string) bool {
			for _, unit := range units {
				if names.UnitTag(unit) == tag {
					return true
				}
			}
			return false
		}, nil
	}
	return &DeployerAPI{
		Remover:         common.NewRemover(st, true, getAuthFunc),
		PasswordChanger: common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:      common.NewLifeGetter(st, getAuthFunc),
		Addresser:       common.NewAddresser(st),
		st:              st,
		resources:       resources,
		authorizer:      authorizer,
	}, nil
}

func (d *DeployerAPI) watchOneMachineUnits(entity params.Entity) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	if !d.authorizer.AuthOwner(entity.Tag) {
		return nothing, common.ErrPerm
	}
	_, id, err := names.ParseTag(entity.Tag, names.MachineTagKind)
	if err != nil {
		return nothing, err
	}
	machine, err := d.st.Machine(id)
	if err != nil {
		return nothing, err
	}
	watch := machine.WatchUnits()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: d.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.MustErr(watch)
}

// WatchUnits starts a StringsWatcher to watch all units deployed to
// any machine passed in args, in order to track which ones should be
// deployed or recalled.
func (d *DeployerAPI) WatchUnits(args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		entityResult, err := d.watchOneMachineUnits(entity)
		result.Results[i] = entityResult
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
