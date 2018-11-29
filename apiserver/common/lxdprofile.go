// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/loggo"
	names "gopkg.in/juju/names.v2"
)

//go:generate mockgen -package mocks -destination mocks/lxdprofile.go github.com/juju/juju/apiserver/common LXDProfileBackend,LXDProfileMachine,LXDProfileUnit

type LXDProfileBackend interface {
	Machine(string) (LXDProfileMachine, error)
	Unit(string) (LXDProfileUnit, error)
}

// LXDProfileMachine describes machine-receiver state methods
// for executing a series upgrade.
type LXDProfileMachine interface {
	WatchLXDProfileUpgradeNotifications() (state.NotifyWatcher, error)
	Units() ([]LXDProfileUnit, error)
	RemoveUpgradeCharmProfileData() error
}

// LXDProfileUnit describes unit-receiver state methods
// for executing a series upgrade.
type LXDProfileUnit interface {
	Tag() names.Tag
	AssignedMachineId() (string, error)
	LXDProfileStatus() (string, error)
}

type LXDProfileAPI struct {
	backend   LXDProfileBackend
	resources facade.Resources

	logger loggo.Logger

	accessUnitOrMachine GetAuthFunc
	AccessMachine       GetAuthFunc
	accessUnit          GetAuthFunc
}

// NewLXDProfileAPI returns a new LXDProfileAPI. Currently both
// GetAuthFuncs can used to determine current permissions.
func NewLXDProfileAPI(
	backend LXDProfileBackend,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessMachine GetAuthFunc,
	accessUnit GetAuthFunc,
	logger loggo.Logger,
) *LXDProfileAPI {
	logger.Tracef("NewLXDProfileAPI called with %s", authorizer.GetAuthTag())
	return &LXDProfileAPI{
		backend:             backend,
		resources:           resources,
		accessUnitOrMachine: AuthAny(accessUnit, accessMachine),
		AccessMachine:       accessMachine,
		accessUnit:          accessUnit,
		logger:              logger,
	}
}

// LXDProfileState implements the LXDProfileBackend indirection
// over state.State.
type LXDProfileState struct {
	st *state.State
}

func (s LXDProfileState) Machine(id string) (LXDProfileMachine, error) {
	m, err := s.st.Machine(id)
	return &lxdProfileMachine{m}, err
}

func (s LXDProfileState) Unit(id string) (LXDProfileUnit, error) {
	return s.st.Unit(id)
}

type lxdProfileMachine struct {
	*state.Machine
}

// Units maintains the UpgradeSeriesMachine indirection by wrapping the call to
// state.Machine.Units().
func (m *lxdProfileMachine) Units() ([]LXDProfileUnit, error) {
	units, err := m.Machine.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}

	wrapped := make([]LXDProfileUnit, len(units))
	for i, u := range units {
		wrapped[i] = u
	}
	return wrapped, nil
}

// NewExternalLXDProfileAPI can be used for API registration.
func NewExternalLXDProfileAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessMachine GetAuthFunc,
	accessUnit GetAuthFunc,
	logger loggo.Logger,
) *LXDProfileAPI {
	return NewLXDProfileAPI(
		LXDProfileState{st},
		resources,
		authorizer,
		accessMachine, accessUnit,
		logger,
	)
}

// WatchLXDProfileUpgradeNotifications returns a NotifyWatcher for observing changes to upgrade series locks.
func (u *LXDProfileAPI) WatchLXDProfileUpgradeNotifications(args params.Entities) (params.NotifyWatchResults, error) {
	u.logger.Tracef("Starting WatchLXDProfileUpgradeNotifications with %+v", args)
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

		if !canAccess(tag) {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		machine, err := u.getMachine(tag)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		w, err := machine.WatchLXDProfileUpgradeNotifications()
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		watcherId := u.resources.Register(w)
		result.Results[i].NotifyWatcherId = watcherId
	}
	return result, nil
}

// LXDProfileUnitStatus returns the current preparation status of an
// upgrading unit.
// If no series upgrade is in progress an error is returned instead.
func (u *LXDProfileAPI) LXDProfileUnitStatus(args params.Entities) (params.LXDProfileStatusResults, error) {
	u.logger.Tracef("Starting LXDProfileUnitStatus with %+v", args)
	return u.unitStatus(args)
}

// RemoveUpgradeCharmProfileData is intended to clean up the LXDProfile status
// to ensure that we start from a clean slate.
func (u *LXDProfileAPI) RemoveUpgradeCharmProfileData(args params.Entities) (params.ErrorResults, error) {
	u.logger.Tracef("Starting RemoveUpgradeCharmProfileData with %+v", args)
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnitOrMachine()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		machine, err := u.getMachine(tag)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		err = machine.RemoveUpgradeCharmProfileData()
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
	}
	return result, nil
}

func (u *LXDProfileAPI) getMachine(tag names.Tag) (LXDProfileMachine, error) {
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

func (u *LXDProfileAPI) getUnit(tag names.Tag) (LXDProfileUnit, error) {
	return u.backend.Unit(tag.Id())
}

func (u *LXDProfileAPI) unitStatus(args params.Entities) (params.LXDProfileStatusResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.LXDProfileStatusResults{}, err
	}

	results := make([]params.LXDProfileStatusResult, len(args.Entities))
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			results[i].Error = ServerError(ErrPerm)
			continue
		}
		if !canAccess(tag) {
			results[i].Error = ServerError(ErrPerm)
			continue
		}
		unit, err := u.getUnit(tag)
		if err != nil {
			results[i].Error = ServerError(err)
			continue
		}
		status, err := unit.LXDProfileStatus()
		if err != nil {
			results[i].Error = ServerError(err)
			continue
		}
		results[i].Status = status
	}
	return params.LXDProfileStatusResults{Results: results}, nil
}
