// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// TODO (manadart 2020-10-21): Remove the ModelUUID method
// from the next version of this facade.

// DeployerAPI provides access to the Deployer API facade.
type DeployerAPI struct {
	*common.Remover
	*common.PasswordChanger
	*common.LifeGetter
	*common.APIAddresser
	*common.UnitsWatcher
	*common.StatusSetter

	st         *state.State
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewDeployerAPI creates a new server-side DeployerAPI facade.
func NewDeployerAPI(ctx facade.Context) (*DeployerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	resources := ctx.Resources()
	leadershipRevoker, err := ctx.LeadershipRevoker(st.ModelUUID())
	if err != nil {
		return nil, errors.Annotate(err, "getting leadership client")
	}
	getAuthFunc := func() (common.AuthFunc, error) {
		// Get all units of the machine and cache them.
		thisMachineTag := authorizer.GetAuthTag()
		units, err := getAllUnits(st, thisMachineTag)
		if err != nil {
			return nil, err
		}
		// Then we just check if the unit is already known.
		return func(tag names.Tag) bool {
			for _, unit := range units {
				// TODO (thumper): remove the names.Tag conversion when gccgo
				// implements concrete-type-to-interface comparison correctly.
				if names.Tag(names.NewUnitTag(unit)) == tag {
					return true
				}
			}
			return false
		}, nil
	}
	getCanWatch := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &DeployerAPI{
		Remover:         common.NewRemover(st, common.RevokeLeadershipFunc(leadershipRevoker), true, getAuthFunc),
		PasswordChanger: common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:      common.NewLifeGetter(st, getAuthFunc),
		APIAddresser:    common.NewAPIAddresser(systemState, resources),
		UnitsWatcher:    common.NewUnitsWatcher(st, resources, getCanWatch),
		StatusSetter:    common.NewStatusSetter(st, getAuthFunc),
		st:              st,
		resources:       resources,
		authorizer:      authorizer,
	}, nil
}

// ConnectionInfo returns all the address information that the
// deployer task needs in one call.
func (d *DeployerAPI) ConnectionInfo() (result params.DeployerConnectionValues, err error) {
	apiAddrs, err := d.APIAddresses()
	if err != nil {
		return result, err
	}
	result = params.DeployerConnectionValues{
		APIAddresses: apiAddrs.Result,
	}
	return result, err
}

// SetStatus sets the status of the specified entities.
func (d *DeployerAPI) SetStatus(args params.SetStatus) (params.ErrorResults, error) {
	return d.StatusSetter.SetStatus(args)
}

// ModelUUID returns the model UUID that this facade is deploying into.
// It is implemented here directly as a result of removing it from
// embedded APIAddresser *without* bumping the facade version.
// It should be blanked when this facade version is next incremented.
func (d *DeployerAPI) ModelUUID() params.StringResult {
	return params.StringResult{Result: d.st.ModelUUID()}
}

// getAllUnits returns a list of all principal and subordinate units
// assigned to the given machine.
func getAllUnits(st *state.State, tag names.Tag) ([]string, error) {
	machine, err := st.Machine(tag.Id())
	if err != nil {
		return nil, err
	}
	// Start a watcher on machine's units, read the initial event and stop it.
	watch := machine.WatchUnits()
	defer func() { _ = watch.Stop() }()
	if units, ok := <-watch.Changes(); ok {
		return units, nil
	}
	return nil, fmt.Errorf("cannot obtain units of machine %q: %v", tag, watch.Err())
}
