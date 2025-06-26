// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error)
	// GetInstanceID returns the cloud specific instance id for this machine.
	GetInstanceID(ctx context.Context, mUUID machine.UUID) (instance.Id, error)
	// GetMachineLife returns the lifecycle state of the machine with the
	// specified name.
	GetMachineLife(ctx context.Context, name machine.Name) (life.Value, error)
	// WatchMachineLife returns a watcher that observes the changes to life of
	// one machine.
	WatchMachineLife(ctx context.Context, name machine.Name) (watcher.NotifyWatcher, error)
}

// ApplicationService defines the methods that the facade assumes from the
// Application service.
type ApplicationService interface {
	// GetUnitLife looks up the life of the specified unit, returning an error
	// satisfying [applicationerrors.UnitNotFoundError] if the unit is not found.
	GetUnitLife(ctx context.Context, unitName unit.Name) (life.Value, error)
	// GetApplicationLifeByName looks up the life of the specified application, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application is not found.
	GetApplicationLifeByName(ctx context.Context, appName string) (life.Value, error)
	// WatchUnitLife returns a watcher that observes the changes to life of one unit.
	WatchUnitLife(ctx context.Context, unitName unit.Name) (watcher.NotifyWatcher, error)
}

// Backend represents the interface required for this facade to retried entity
// information.
type Backend interface {
	state.EntityFinder
}

// NewFacade constructs a new life flag facade.
func NewFacade(
	applicationService ApplicationService,
	machineService MachineService,
	backend Backend,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	logger logger.Logger,
) (*Facade, error) {
	if !authorizer.AuthUnitAgent() && authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	getCanAccess := func(context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return authorizer.AuthOwner(tag)
		}, nil
	}
	life := common.NewLifeGetter(applicationService, machineService, backend, getCanAccess, logger)
	return &Facade{
		LifeGetter:         life,
		watcherRegistry:    watcherRegistry,
		authorizer:         authorizer,
		applicationService: applicationService,
	}, nil
}

type Facade struct {
	*common.LifeGetter
	watcherRegistry facade.WatcherRegistry
	authorizer      facade.Authorizer

	applicationService ApplicationService
	machineService     MachineService
}

// Watch starts an NotifyWatcher for each given entity.
func (a *Facade) Watch(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
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

		if !a.authorizer.AuthOwner(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		switch tag := tag.(type) {
		case names.UnitTag:
			watcherID, err := a.watchUnit(ctx, unit.Name(tag.Id()))
			result.Results[i] = params.NotifyWatchResult{
				NotifyWatcherId: watcherID,
				Error:           apiservererrors.ServerError(err),
			}

		case names.MachineTag:
			watcherID, err := a.watchMachine(ctx, machine.Name(tag.Id()))
			result.Results[i] = params.NotifyWatchResult{
				NotifyWatcherId: watcherID,
				Error:           apiservererrors.ServerError(err),
			}

		default:
			result.Results[i].Error = apiservererrors.ServerError(
				errors.NotImplementedf("agent type of %s", tag.Kind()),
			)
		}
	}
	return result, nil
}

func (a *Facade) watchUnit(ctx context.Context, unitName unit.Name) (string, error) {
	watch, err := a.applicationService.WatchUnitLife(ctx, unitName)
	if err != nil {
		return "", err
	}
	id, _, err := internal.EnsureRegisterWatcher(ctx, a.watcherRegistry, watch)
	if err != nil {
		return "", errors.Trace(err)
	}
	return id, nil
}

func (a *Facade) watchMachine(ctx context.Context, machineName machine.Name) (string, error) {
	watch, err := a.machineService.WatchMachineLife(ctx, machineName)
	if err != nil {
		return "", err
	}
	id, _, err := internal.EnsureRegisterWatcher(ctx, a.watcherRegistry, watch)
	if err != nil {
		return "", errors.Trace(err)
	}
	return id, nil
}
