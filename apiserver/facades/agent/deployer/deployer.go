// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/objectstore"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// TODO (manadart 2020-10-21): Remove the ModelUUID method
// from the next version of this facade.

// AgentPasswordService defines the methods required to set an agent password
// hash.
type AgentPasswordService interface {
	// SetUnitPassword sets the password hash for the given unit.
	SetUnitPassword(context.Context, coreunit.Name, string) error
}

// ControllerConfigGetter is the interface that the facade needs to get controller config.
type ControllerConfigGetter interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ApplicationService removes a unit from the dqlite database.
type ApplicationService interface {
	GetUnitLife(context.Context, coreunit.Name) (life.Value, error)
	EnsureUnitDead(context.Context, coreunit.Name, leadership.Revoker) error
	RemoveUnit(context.Context, coreunit.Name, leadership.Revoker) error
}

type StatusService interface {
	// GetUnitWorkloadStatus returns the workload status of the specified unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
	GetUnitWorkloadStatus(context.Context, coreunit.Name) (corestatus.StatusInfo, error)

	// SetUnitWorkloadStatus sets the workload status of the specified unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
	SetUnitWorkloadStatus(context.Context, coreunit.Name, corestatus.StatusInfo) error
}

// DeployerAPI provides access to the Deployer API facade.
type DeployerAPI struct {
	*common.PasswordChanger
	*common.APIAddresser
	*common.UnitsWatcher
	unitStatusSetter *common.UnitStatusSetter

	canRead  func(tag names.Tag) bool
	canWrite func(tag names.Tag) bool

	controllerConfigGetter ControllerConfigGetter
	applicationService     ApplicationService
	leadershipRevoker      leadership.Revoker

	store      objectstore.ObjectStore
	st         *state.State
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewDeployerAPI creates a new server-side DeployerAPI facade.
func NewDeployerAPI(
	agentPasswordService AgentPasswordService,
	controllerConfigGetter ControllerConfigGetter,
	applicationService ApplicationService,
	statusService StatusService,
	authorizer facade.Authorizer,
	st *state.State,
	store objectstore.ObjectStore,
	resources facade.Resources,
	leadershipRevoker leadership.Revoker,
	systemState *state.State,
	clock clock.Clock,
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
	auth, err := getAuthFunc()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &DeployerAPI{
		PasswordChanger:        common.NewPasswordChanger(agentPasswordService, st, getAuthFunc),
		APIAddresser:           common.NewAPIAddresser(systemState, resources),
		UnitsWatcher:           common.NewUnitsWatcher(st, resources, getCanWatch),
		unitStatusSetter:       common.NewUnitStatusSetter(statusService, clock, getAuthFunc),
		controllerConfigGetter: controllerConfigGetter,
		applicationService:     applicationService,
		leadershipRevoker:      leadershipRevoker,
		canRead:                auth,
		canWrite:               auth,
		store:                  store,
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
	return d.unitStatusSetter.SetStatus(ctx, args)
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

// Life returns the life of the specified units.
func (d *DeployerAPI) Life(ctx context.Context, args params.Entities) (params.LifeResults, error) {
	result := params.LifeResults{
		Results: make([]params.LifeResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !d.canRead(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		lifeValue, err := d.applicationService.GetUnitLife(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			err = errors.NotFoundf("unit %s", unitName)
		}
		result.Results[i].Life = lifeValue
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
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

// Remove removes every given unit from state, calling EnsureDead
// first, then Remove.
func (d *DeployerAPI) Remove(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !d.canWrite(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		// TODO(units) - remove me.
		// Dual write to state.
		// We need to set the unit life to Dead in state **first**
		// because the life watcher is currently looking at state
		// not dqlite.
		unit, err := d.st.Unit(tag.Id())
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				err = apiservererrors.ErrPerm
			}
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if unit.Life() == state.Alive {
			result.Results[i].Error = apiservererrors.ServerError(errors.Errorf("cannot remove unit %q: still alive", tag.Id()))
			continue
		}
		if err := unit.EnsureDead(); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// Given the way dual write works, we need this for now.
		if err = d.applicationService.EnsureUnitDead(ctx, unitName, d.leadershipRevoker); err != nil {
			if errors.Is(err, applicationerrors.UnitNotFound) {
				err = errors.NotFoundf("unit %s", unitName)
			}
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// This is the call we will keep once mongo is removed.
		// We will need to remove the alive check.
		if err = d.applicationService.RemoveUnit(ctx, unitName, d.leadershipRevoker); err != nil {
			if errors.Is(err, applicationerrors.UnitNotFound) {
				err = errors.NotFoundf("unit %s", unitName)
			}
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// TODO(units) - remove me.
		if err := unit.Remove(d.store); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}
