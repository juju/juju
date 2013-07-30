// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
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
	id, err := names.MachineIdFromTag(machineTag)
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
		Remover:         common.NewRemover(st, getAuthFunc),
		PasswordChanger: common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:      common.NewLifeGetter(st, getAuthFunc),
		st:              st,
		resources:       resources,
		authorizer:      authorizer,
	}, nil
}

func (d *DeployerAPI) watchOneMachineUnits(entity params.Entity) (*params.StringsWatchResult, error) {
	if !d.authorizer.AuthOwner(entity.Tag) {
		return nil, common.ErrPerm
	}
	id, err := names.MachineIdFromTag(entity.Tag)
	if err != nil {
		return nil, err
	}
	machine, err := d.st.Machine(id)
	if err != nil {
		return nil, err
	}
	watch := machine.WatchUnits()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return &params.StringsWatchResult{
			StringsWatcherId: d.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nil, watcher.MustErr(watch)
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
		if err == nil {
			result.Results[i] = *entityResult
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// getEnvironStateInfo returns the state and API connection
// information from the state and the environment.
//
// TODO(dimitern): Remove this once we have a way to get state/API
// public addresses from state.
// BUG(lp:1205371): This is temporary, until the Addresser worker
// lands and we can take the addresses of all machines with
// JobManageState.
func (d *DeployerAPI) getEnvironStateInfo() (*state.Info, *api.Info, error) {
	cfg, err := d.st.EnvironConfig()
	if err != nil {
		return nil, nil, err
	}
	env, err := environs.New(cfg)
	if err != nil {
		return nil, nil, err
	}
	return env.StateInfo()
}

// StateAddresses returns the list of addresses used to connect to the state.
//
// TODO(dimitern): Remove this once we have a way to get state/API
// public addresses from state.
// BUG(lp:1205371): This is temporary, until the Addresser worker
// lands and we can take the addresses of all machines with
// JobManageState.
func (d *DeployerAPI) StateAddresses() (params.StringsResult, error) {
	stateInfo, _, err := d.getEnvironStateInfo()
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: stateInfo.Addrs,
	}, nil
}

// APIAddresses returns the list of addresses used to connect to the API.
//
// TODO(dimitern): Remove this once we have a way to get state/API
// public addresses from state.
// BUG(lp:1205371): This is temporary, until the Addresser worker
// lands and we can take the addresses of all machines with
// JobManageState.
func (d *DeployerAPI) APIAddresses() (params.StringsResult, error) {
	_, apiInfo, err := d.getEnvironStateInfo()
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: apiInfo.Addrs,
	}, nil
}

// CACert returns the certificate used to validate the state connection.
func (d *DeployerAPI) CACert() params.BytesResult {
	return params.BytesResult{
		Result: d.st.CACert(),
	}
}
