// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/upgradeseries.go github.com/juju/juju/apiserver/common UpgradeSeriesBackend,UpgradeSeriesMachine,UpgradeSeriesUnit

type UpgradeSeriesBackend interface {
	Machine(string) (UpgradeSeriesMachine, error)
	Unit(string) (UpgradeSeriesUnit, error)
}

// UpgradeSeriesMachine describes machine-receiver state methods
// for executing a series upgrade.
type UpgradeSeriesMachine interface {
	WatchUpgradeSeriesNotifications() (state.NotifyWatcher, error)
	Units() ([]UpgradeSeriesUnit, error)
	UpgradeSeriesStatus() (model.UpgradeSeriesStatus, error)
	SetUpgradeSeriesStatus(model.UpgradeSeriesStatus, string) error
	StartUpgradeSeriesUnitCompletion(string) error
	UpgradeSeriesUnitStatuses() (map[string]state.UpgradeSeriesUnitStatus, error)
	RemoveUpgradeSeriesLock() error
	UpgradeSeriesTarget() (string, error)
	Base() state.Base
	UpdateMachineSeries(base state.Base) error
	SetInstanceStatus(status.StatusInfo, status.StatusHistoryRecorder) error
}

// UpgradeSeriesUnit describes unit-receiver state methods
// for executing a series upgrade.
type UpgradeSeriesUnit interface {
	Tag() names.Tag
	AssignedMachineId() (string, error)
	UpgradeSeriesStatus() (model.UpgradeSeriesStatus, string, error)
	SetUpgradeSeriesStatus(model.UpgradeSeriesStatus, string) error
}

// UpgradeSeriesState implements the UpgradeSeriesBackend indirection
// over state.State.
type UpgradeSeriesState struct {
	St *state.State
}

func (s UpgradeSeriesState) Machine(id string) (UpgradeSeriesMachine, error) {
	m, err := s.St.Machine(id)
	return &upgradeSeriesMachine{m}, err
}

func (s UpgradeSeriesState) Unit(id string) (UpgradeSeriesUnit, error) {
	return s.St.Unit(id)
}

type upgradeSeriesMachine struct {
	*state.Machine
}

// Units maintains the UpgradeSeriesMachine indirection by wrapping the call to
// state.Machine.Units().
func (m *upgradeSeriesMachine) Units() ([]UpgradeSeriesUnit, error) {
	units, err := m.Machine.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}

	wrapped := make([]UpgradeSeriesUnit, len(units))
	for i, u := range units {
		wrapped[i] = u
	}
	return wrapped, nil
}

type UpgradeSeriesAPI struct {
	backend   UpgradeSeriesBackend
	resources facade.Resources

	logger loggo.Logger

	accessUnitOrMachine GetAuthFunc
	AccessMachine       GetAuthFunc
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
		AccessMachine:       accessMachine,
		accessUnit:          accessUnit,
		logger:              logger,
	}
}

// WatchUpgradeSeriesNotifications returns a NotifyWatcher for observing changes to upgrade series locks.
func (u *UpgradeSeriesAPI) WatchUpgradeSeriesNotifications(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
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
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machine, err := u.GetMachine(ctx, tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		w, err := machine.WatchUpgradeSeriesNotifications()
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		watcherId := u.resources.Register(w)
		result.Results[i].NotifyWatcherId = watcherId
	}
	return result, nil
}

// UpgradeSeriesUnitStatus returns the current preparation status of an
// upgrading unit.
// If no series upgrade is in progress an error is returned instead.
func (u *UpgradeSeriesAPI) UpgradeSeriesUnitStatus(ctx context.Context, args params.Entities) (params.UpgradeSeriesStatusResults, error) {
	u.logger.Tracef("Starting UpgradeSeriesUnitStatus with %+v", args)
	return u.unitStatus(args)
}

// SetUpgradeSeriesUnitStatus sets the upgrade series status of the unit.
// If no upgrade is in progress an error is returned instead.
func (u *UpgradeSeriesAPI) SetUpgradeSeriesUnitStatus(
	ctx context.Context,
	args params.UpgradeSeriesStatusParams,
) (params.ErrorResults, error) {
	u.logger.Tracef("Starting SetUpgradeSeriesUnitStatus with %+v", args)
	return u.setUnitStatus(args)
}

func (u *UpgradeSeriesAPI) GetMachine(ctx context.Context, tag names.Tag) (UpgradeSeriesMachine, error) {
	var id string
	switch tag.Kind() {
	case names.MachineTagKind:
		id = tag.Id()
	case names.UnitTagKind:
		unit, err := u.backend.Unit(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		id, err = unit.AssignedMachineId()
		if err != nil {
			return nil, errors.Trace(err)
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
	return NewUpgradeSeriesAPI(UpgradeSeriesState{st}, resources, authorizer, accessMachine, accessUnit, logger)
}

func (u *UpgradeSeriesAPI) setUnitStatus(args params.UpgradeSeriesStatusParams) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Params)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, p := range args.Params {
		tag, err := names.ParseUnitTag(p.Entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unit, err := u.getUnit(tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		graph := model.UpgradeSeriesGraph()
		if !graph.ValidState(p.Status) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotValidf("upgrade series status %q", p.Status))
			continue
		}

		sts, _, err := unit.UpgradeSeriesStatus()
		if err != nil {
			logger.Tracef("unit upgrade series status not found, fallback to not-started: %v", err)
			sts = model.UpgradeSeriesNotStarted
		}
		if !graph.ValidState(sts) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotValidf("current upgrade series status %q", sts))
			continue
		}

		// If attempting to set the same status, we're done.
		// This can happen in situations where the upgrade completion hook
		// fails and requires resolution before re-running.
		if sts == p.Status {
			logger.Debugf("unit %s already has upgrade series status %s", tag.Id(), sts)
			continue
		}

		fsm, err := model.NewUpgradeSeriesFSM(graph, sts)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !fsm.TransitionTo(p.Status) {
			result.Results[i].Error = apiservererrors.ServerError(errors.BadRequestf("upgrade series status %q", p.Status))
			continue
		}

		if err = unit.SetUpgradeSeriesStatus(p.Status, p.Message); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

func (u *UpgradeSeriesAPI) unitStatus(args params.Entities) (params.UpgradeSeriesStatusResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.UpgradeSeriesStatusResults{}, err
	}

	results := make([]params.UpgradeSeriesStatusResult, len(args.Entities))
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unit, err := u.getUnit(tag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		status, target, err := unit.UpgradeSeriesStatus()
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results[i].Status = status
		results[i].Target = target
	}
	return params.UpgradeSeriesStatusResults{Results: results}, nil
}
