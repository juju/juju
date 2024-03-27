// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// TODO (manadart 2020-10-21): Remove the ModelUUID method
// from the next version of this facade.

// ControllerConfigGetter is the interface that the facade needs to get controller config.
type ControllerConfigGetter interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// UnitRemover deletes a unit from the dqlite database.
// This allows us to initially weave some dqlite support into the cleanup workflow.
type UnitRemover interface {
	DeleteUnit(context.Context, string) error
}

// DeployerAPI provides access to the Deployer API facade.
type DeployerAPI struct {
	*common.Remover
	*common.PasswordChanger
	*common.LifeGetter
	*common.APIAddresser
	*common.UnitsWatcher
	*common.StatusSetter

	controllerConfigGetter ControllerConfigGetter
	st                     *state.State
	resources              facade.Resources
	authorizer             facade.Authorizer
}

// NewDeployerAPI creates a new server-side DeployerAPI facade.
func NewDeployerAPI(
	controllerConfigGetter ControllerConfigGetter,
	unitRemover UnitRemover,
	authorizer facade.Authorizer,
	st *state.State,
	store objectstore.ObjectStore,
	resources facade.Resources,
	leadershipRevoker leadership.Revoker,
	systemState *state.State,
	statusHistory status.StatusHistoryRecorder,
) (*DeployerAPI, error) {
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

	return &DeployerAPI{
		Remover:                common.NewRemover(st, store, common.RevokeLeadershipFunc(leadershipRevoker), true, getAuthFunc, unitRemover),
		PasswordChanger:        common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:             common.NewLifeGetter(st, getAuthFunc),
		APIAddresser:           common.NewAPIAddresser(systemState, resources),
		UnitsWatcher:           common.NewUnitsWatcher(st, resources, getCanWatch),
		StatusSetter:           common.NewStatusSetter(st, getAuthFunc, statusHistory),
		controllerConfigGetter: controllerConfigGetter,
		st:                     st,
		resources:              resources,
		authorizer:             authorizer,
	}, nil
}

// ConnectionInfo returns all the address information that the
// deployer task needs in one call.
func (d *DeployerAPI) ConnectionInfo(ctx context.Context) (result params.DeployerConnectionValues, err error) {
	apiAddrs, err := d.APIAddresses(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	result = params.DeployerConnectionValues{
		APIAddresses: apiAddrs.Result,
	}
	return result, nil
}

// SetStatus sets the status of the specified entities.
func (d *DeployerAPI) SetStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	return d.StatusSetter.SetStatus(ctx, args)
}

// ModelUUID returns the model UUID that this facade is deploying into.
// It is implemented here directly as a result of removing it from
// embedded APIAddresser *without* bumping the facade version.
// It should be blanked when this facade version is next incremented.
func (d *DeployerAPI) ModelUUID() params.StringResult {
	return params.StringResult{Result: d.st.ModelUUID()}
}

// APIHostPorts returns the API server addresses.
func (d *DeployerAPI) APIHostPorts(ctx context.Context) (result params.APIHostPortsResult, err error) {
	controllerConfig, err := d.controllerConfigGetter.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	return d.APIAddresser.APIHostPorts(ctx, controllerConfig)
}

// APIAddresses returns the list of addresses used to connect to the API.
func (d *DeployerAPI) APIAddresses(ctx context.Context) (result params.StringsResult, err error) {
	controllerConfig, err := d.controllerConfigGetter.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	return d.APIAddresser.APIAddresses(ctx, controllerConfig)
}

// getAllUnits returns a list of all principal and subordinate units
// assigned to the given machine.
func getAllUnits(st *state.State, tag names.Tag) ([]string, error) {
	machine, err := st.Machine(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Start a watcher on machine's units, read the initial event and stop it.
	watch := machine.WatchUnits()
	defer func() { _ = watch.Stop() }()
	if units, ok := <-watch.Changes(); ok {
		return units, nil
	}
	return nil, errors.Errorf("cannot obtain units of machine %q: %v", tag, watch.Err())
}
