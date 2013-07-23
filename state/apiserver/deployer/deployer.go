// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/utils/set"
)

// DeployerAPI provides access to the Deployer API facade.
type DeployerAPI struct {
	*common.Remover
	*common.PasswordChanger
	*common.LifeGetter

	st         *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

// getAllUnits returns a list of all principal and subordinate units
// assigned to the given machine.
func getAllUnits(st *state.State, machineTag string) ([]string, error) {
	machine, err := st.Machine(state.MachineIdFromTag(machineTag))
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

// NewDeployerAPI creates a new client-side DeployerAPI facade.
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
		knownUnits := set.NewStrings()
		thisMachineTag := authorizer.GetAuthTag()
		if units, err := getAllUnits(st, thisMachineTag); err != nil {
			return nil, err
		} else {
			for _, unit := range units {
				knownUnits.Add(unit)
			}
		}
		// Then we just check if the unit is already known.
		return func(tag string) bool {
			unitName := state.UnitNameFromTag(tag)
			return knownUnits.Contains(unitName)
		}, nil
	}
	return &DeployerAPI{
		Remover:         common.NewRemover(st, getAuthFunc),
		PasswordChanger: common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:      common.NewLifeGetter(st, getAuthFunc),
		st:              st,
		resources:       resources,
		authorizer:      authorizer,
	}, nil
}

// WatchUnits starts a StringsWatcher to watch all units deployed to
// any machine passed in args, in order to track which ones should be
// deployed or recalled.
func (d *DeployerAPI) WatchUnits(args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if d.authorizer.AuthOwner(entity.Tag) {
			var machine *state.Machine
			machine, err = d.st.Machine(state.MachineIdFromTag(entity.Tag))
			if err == nil {
				watch := machine.WatchUnits()
				// Consume the initial event and forward it to the result.
				if changes, ok := <-watch.Changes(); ok {
					result.Results[i].StringsWatcherId = d.resources.Register(watch)
					result.Results[i].Changes = changes
				} else {
					err = watcher.MustErr(watch)
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CanDeploy returns if the currently authenticated entity (a machine
// agent) can deploy each passed unit entity.
func (d *DeployerAPI) CanDeploy(args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		unitName := state.UnitNameFromTag(entity.Tag)
		unit, err := d.st.Unit(unitName)
		if err == nil {
			machineId, err := unit.AssignedMachineId()
			if err == nil {
				if machineId != "" {
					machineTag := state.MachineTag(machineId)
					if d.authorizer.AuthOwner(machineTag) {
						result.Results[i].Result = true
					}
				}
			}
		}
		if !result.Results[i].Result {
			// If unassigned, not assigned to the deployer's
			// machine, or on any other error, deny access.
			result.Results[i].Error = common.ServerError(common.ErrPerm)
		}
	}
	return result, nil
}
