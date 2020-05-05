// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/lxdprofile.go github.com/juju/juju/apiserver/facades/agent/uniter LXDProfileBackend,LXDProfileMachine,LXDProfileUnit

type LXDProfileBackend interface {
	Machine(string) (LXDProfileMachine, error)
	Unit(string) (LXDProfileUnit, error)
}

// LXDProfileMachine describes machine-receiver state methods
// for executing a lxd profile upgrade.
type LXDProfileMachine interface {
	WatchLXDProfileUpgradeNotifications(string) (state.StringsWatcher, error)
}

// LXDProfileUnit describes unit-receiver state methods
// for executing a lxd profile upgrade.
type LXDProfileUnit interface {
	AssignedMachineId() (string, error)
	Name() string
	Tag() names.Tag
	WatchLXDProfileUpgradeNotifications() (state.StringsWatcher, error)
}

type LXDProfileAPI struct {
	backend   LXDProfileBackend
	resources facade.Resources

	logger     loggo.Logger
	accessUnit common.GetAuthFunc
}

// NewLXDProfileAPI returns a new LXDProfileAPI. Currently both
// GetAuthFuncs can used to determine current permissions.
func NewLXDProfileAPI(
	backend LXDProfileBackend,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessUnit common.GetAuthFunc,
	logger loggo.Logger,
) *LXDProfileAPI {
	logger.Tracef("NewLXDProfileAPI called with %s", authorizer.GetAuthTag())
	return &LXDProfileAPI{
		backend:    backend,
		resources:  resources,
		accessUnit: accessUnit,
		logger:     logger,
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

// NewExternalLXDProfileAPI can be used for API registration.
func NewExternalLXDProfileAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessUnit common.GetAuthFunc,
	logger loggo.Logger,
) *LXDProfileAPI {
	return NewLXDProfileAPI(
		LXDProfileState{st},
		resources,
		authorizer,
		accessUnit,
		logger,
	)
}

// WatchUnitLXDProfileUpgradeNotifications returns a StringsWatcher for observing
// changes to the lxd profile changes for one unit.
func (u *LXDProfileAPI) WatchUnitLXDProfileUpgradeNotifications(args params.Entities) (params.StringsWatchResults, error) {
	u.logger.Tracef("Starting WatchUnitLXDProfileUpgradeNotifications with %+v", args)
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		unit, err := u.getLXDProfileUnit(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		watcherId, initial, err := u.watchOneChangeUnitLXDProfileUpgradeNotifications(unit)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		result.Results[i].StringsWatcherId = watcherId
		result.Results[i].Changes = initial
	}
	return result, nil
}

func (u *LXDProfileAPI) watchOneChangeUnitLXDProfileUpgradeNotifications(unit LXDProfileUnit) (string, []string, error) {
	watch, err := unit.WatchLXDProfileUpgradeNotifications()
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	if changes, ok := <-watch.Changes(); ok {
		return u.resources.Register(watch), changes, nil
	}
	return "", nil, watcher.EnsureErr(watch)
}

// WatchLXDProfileUpgradeNotifications returns a StringsWatcher for observing
// changes to the lxd profile changes.
//
// NOTE: can be removed in juju version 3.
func (u *LXDProfileAPI) WatchLXDProfileUpgradeNotifications(args params.LXDProfileUpgrade) (params.StringsWatchResults, error) {
	u.logger.Tracef("Starting WatchLXDProfileUpgradeNotifications with %+v", args)
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := u.getLXDProfileMachine(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		watcherId, initial, err := u.watchOneChangeLXDProfileUpgradeNotifications(machine, args.ApplicationName)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		result.Results[i].StringsWatcherId = watcherId
		result.Results[i].Changes = initial
	}
	return result, nil
}

func (u *LXDProfileAPI) watchOneChangeLXDProfileUpgradeNotifications(machine LXDProfileMachine, applicationName string) (string, []string, error) {
	watch, err := machine.WatchLXDProfileUpgradeNotifications(applicationName)
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	if changes, ok := <-watch.Changes(); ok {
		return u.resources.Register(watch), changes, nil
	}
	return "", nil, watcher.EnsureErr(watch)
}

// RemoveUpgradeCharmProfileData is intended to clean up the LXDProfile status
// to ensure that we start from a clean slate.
func (u *LXDProfileAPI) RemoveUpgradeCharmProfileData(args params.Entities) (params.ErrorResults, error) {
	// This is a canned response for V9 of the API, so that clients will still
	// be supported and the error for each params entity is nil, along with the
	// call.
	return params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}, nil
}

func (u *LXDProfileAPI) getLXDProfileMachine(tag names.Tag) (LXDProfileMachine, error) {
	var id string
	if tag.Kind() != names.UnitTagKind {
		return nil, errors.Errorf("not a unit tag")
	}
	unit, err := u.backend.Unit(tag.Id())
	if err != nil {
		return nil, err
	}
	id, err = unit.AssignedMachineId()
	if err != nil {
		return nil, err
	}
	return u.backend.Machine(id)
}

func (u *LXDProfileAPI) getLXDProfileUnit(tag names.Tag) (LXDProfileUnit, error) {
	return u.backend.Unit(tag.Id())
}
