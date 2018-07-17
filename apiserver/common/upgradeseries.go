// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
	"github.com/juju/loggo"
)

//go:generate mockgen -package mocks -destination mocks/mock_backend.go github.com/juju/juju/apiserver/common UpgradeSeriesBackend
type UpgradeSeriesBackend interface {
	Machine(string) (UpgradeSeriesMachine, error)
	Unit(string) (UpgradeSeriesUnit, error)
}

//go:generate mockgen -package mocks -destination mocks/mock_machine.go github.com/juju/juju/apiserver/common UpgradeSeriesMachine
type UpgradeSeriesMachine interface {
	WatchUpgradeSeriesNotifications() (state.NotifyWatcher, error)
}

//go:generate mockgen -package mocks -destination mocks/mock_unit.go github.com/juju/juju/apiserver/common UpgradeSeriesUnit
type UpgradeSeriesUnit interface {
	AssignedMachineId() (string, error)
	UpgradeSeriesStatus() (model.UnitSeriesUpgradeStatus, error)
	SetUpgradeSeriesStatus(status model.UnitSeriesUpgradeStatus) error
}

type UpgradeSeriesAPI struct {
	backend   UpgradeSeriesBackend
	resources facade.Resources

	logger loggo.Logger

	accessUnitOrMachine GetAuthFunc
	accessMachine       GetAuthFunc
	accessUnit          GetAuthFunc
}

// NewUpgradeSeriesAPI returns a new UpgradeSeriesAPI. Currently both
// GetAuthFuncs can used to determine current permissions.
func NewUpgradeSeriesAPI(
	backend UpgradeSeriesBackend,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessMachine GetAuthFunc,
	accessUnit GetAuthFunc,
	logger loggo.Logger,
) *UpgradeSeriesAPI {
	logger.Tracef("NewUpgradeSeriesAPI called with %s", authorizer.GetAuthTag())
	return &UpgradeSeriesAPI{
		backend:             backend,
		resources:           resources,
		accessUnitOrMachine: AuthAny(accessUnit, accessMachine),
		accessMachine:       accessMachine,
		accessUnit:          accessUnit,
		logger:              logger,
	}
}

// WatchUpgradeSeriesNotifications returns a NotifyWatcher for observing changes to upgrade series locks.
func (u *UpgradeSeriesAPI) WatchUpgradeSeriesNotifications(args params.Entities) (params.NotifyWatchResults, error) {
	u.logger.Tracef("Starting WatchUpgradeSeriesNotifications with %+v", args)
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnitOrMachine()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}

		watcherId := ""
		if !canAccess(tag) {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		machine, err := u.getMachine(tag)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		w, err := machine.WatchUpgradeSeriesNotifications()
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		watcherId = u.resources.Register(w)
		result.Results[i].NotifyWatcherId = watcherId
	}
	return result, nil
}

// UpgradeSeriesStatus returns the current state of series upgrading
// unit. If no upgrade is in progress an error is returned instead.
func (u *UpgradeSeriesAPI) UpgradeSeriesStatus(args params.Entities) (params.UpgradeSeriesStatusResults, error) {
	u.logger.Tracef("Starting UpgradeSeriesStatus with %+v", args)
	result := params.UpgradeSeriesStatusResults{
		Results: make([]params.UpgradeSeriesStatusResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnitOrMachine()
	if err != nil {
		return params.UpgradeSeriesStatusResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		unit, err := u.getUnit(tag)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		status, err := unit.UpgradeSeriesStatus()
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		result.Results[i].Status = string(status)
	}

	return result, nil
}

// SetUpgradeSeriesStatus sets the upgrade series status of the unit.
// If no upgrade is in progress an error is returned instead.
func (u *UpgradeSeriesAPI) SetUpgradeSeriesStatus(args params.SetUpgradeSeriesStatusParams) (params.ErrorResults, error) {
	u.logger.Tracef("Starting SetUpgradeSeriesStatus with %+v", args)
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Params)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, p := range args.Params {
		//TODO[externalreality] refactor all of this, its being copied often.
		tag, err := names.ParseUnitTag(p.Entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		unit, err := u.getUnit(tag)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		status, err := model.ValidateUnitSeriesUpgradeStatus(p.Status)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		err = unit.SetUpgradeSeriesStatus(status)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
	}

	return result, nil
}

func (u *UpgradeSeriesAPI) getMachine(tag names.Tag) (UpgradeSeriesMachine, error) {
	var id string
	switch tag.Kind() {
	case names.MachineTagKind:
		id = tag.Id()
	case names.UnitTagKind:
		unit, err := u.backend.Unit(tag.Id())
		if err != nil {

		}
		id, err = unit.AssignedMachineId()
		if err != nil {
			return nil, err
		}
	default:
	}
	return u.backend.Machine(id)
}

func (u *UpgradeSeriesAPI) getUnit(tag names.Tag) (UpgradeSeriesUnit, error) {
	return u.backend.Unit(tag.Id())
}

// NewExternalUpgradeSeriesAPI can be used for API registration.
func NewExternalUpgradeSeriesAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessMachine GetAuthFunc,
	accessUnit GetAuthFunc,
	logger loggo.Logger,
) *UpgradeSeriesAPI {
	return NewUpgradeSeriesAPI(backendShim{st}, resources, authorizer, accessMachine, accessUnit, logger)
}

type backendShim struct {
	st *state.State
}

func (shim backendShim) Machine(id string) (UpgradeSeriesMachine, error) {
	return shim.st.Machine(id)
}

func (shim backendShim) Unit(id string) (UpgradeSeriesUnit, error) {
	return shim.st.Unit(id)
}
