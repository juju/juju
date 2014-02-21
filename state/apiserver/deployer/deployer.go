// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"

	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

// DeployerAPI provides access to the Deployer API facade.
type DeployerAPI struct {
	*common.Remover
	*common.PasswordChanger
	*common.LifeGetter
	*common.StateAddresser
	*common.APIAddresser
	*common.UnitsWatcher

	st         *state.State
	resources  *common.Resources
	authorizer common.Authorizer
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
	getCanWatch := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &DeployerAPI{
		Remover:         common.NewRemover(st, true, getAuthFunc),
		PasswordChanger: common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:      common.NewLifeGetter(st, getAuthFunc),
		StateAddresser:  common.NewStateAddresser(st),
		APIAddresser:    common.NewAPIAddresser(st),
		UnitsWatcher:    common.NewUnitsWatcher(st, resources, getCanWatch),
		st:              st,
		resources:       resources,
		authorizer:      authorizer,
	}, nil
}

// ConnectionInfo returns all the address information that the
// deployer task needs in one call.
func (d *DeployerAPI) ConnectionInfo() (result params.DeployerConnectionValues, err error) {
	info, err := d.st.DeployerConnectionInfo()
	if info != nil {
		result = params.DeployerConnectionValues{
			StateAddresses: info.StateAddresses,
			APIAddresses:   info.APIAddresses,
			SyslogPort:     info.SyslogPort,
		}
	}
	return result, err
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
