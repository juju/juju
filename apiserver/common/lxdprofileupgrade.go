// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
	"github.com/juju/loggo"
	names "gopkg.in/juju/names.v2"
)

//go:generate mockgen -package mocks -destination mocks/mock_lxdprofileupgrade.go github.com/juju/juju/apiserver/common LXDProfileUpgradeBackend,LXDProfileUpgradeMachine,LXDProfileUpgradeUnit

type LXDProfileUpgradeBackend interface {
	Machine(string) (LXDProfileUpgradeMachine, error)
	Unit(string) (LXDProfileUpgradeUnit, error)
}

// LXDProfileUpgradeMachine describes machine-receiver state methods.
type LXDProfileUpgradeMachine interface {
	WatchLXDProfileUpgradeNotifications() (state.NotifyWatcher, error)
	LXDProfileUpgradeStatus() (model.LXDProfileUpgradeStatus, error)
}

// LXDProfileUpgradeUnit describes unit-receiver state methods.
type LXDProfileUpgradeUnit interface {
	Tag() names.Tag
	AssignedMachineId() (string, error)
	LXDProfileUpgradeStatus() (model.LXDProfileUpgradeStatus, error)
}

type lxdprofileMachine struct {
	*state.Machine
}

// LXDProfileUpgradeState implements the LXDProfileUpgradeBackend indirection over state.State
type LXDProfileUpgradeState struct {
	St *state.State
}

func (s LXDProfileUpgradeState) Machine(id string) (LXDProfileUpgradeMachine, error) {
	m, err := s.St.Machine(id)
	return &lxdProfileMachine{m}, errors.Trace(err)
}

func (s LXDProfileUpgradeState) Unit(id string) (LXDProfileUpgradeUnit, error) {
	return s.St.Unit(id)
}

type lxdProfileMachine struct {
	*state.Machine
}

type LXDProfileUpgradeAPI struct {
	backend   LXDProfileUpgradeBackend
	resources facade.Resources

	logger loggo.Logger

	accessUnitOrMachine GetAuthFunc
	AccessMachine       GetAuthFunc
	accessUnit          GetAuthFunc
}

// NewLXDProfileUpgradeAPI returns a new LXDProfileUpgradeAPI. Currently both
// GetAuthFuncs can used to determine current permissions.
func NewLXDProfileUpgradeAPI(
	backend LXDProfileUpgradeBackend,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessMachine GetAuthFunc,
	accessUnit GetAuthFunc,
	logger loggo.Logger,
) *LXDProfileUpgradeAPI {
	logger.Tracef("NewLXDProfileUpgradeAPI called with %s", authorizer.GetAuthTag())
	return &LXDProfileUpgradeAPI{
		backend:             backend,
		resources:           resources,
		accessUnitOrMachine: AuthAny(accessUnit, accessMachine),
		AccessMachine:       accessMachine,
		accessUnit:          accessUnit,
		logger:              logger,
	}
}

// NewExternalLXDProfileUpgradeAPI can be used for API registration.
func NewExternalLXDProfileUpgradeAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessMachine GetAuthFunc,
	accessUnit GetAuthFunc,
	logger loggo.Logger,
) *LXDProfileUpgradeAPI {
	return NewLXDProfileUpgradeAPI(LXDProfileUpgradeState{st}, resources, authorizer, accessMachine, accessUnit, logger)
}

// WatchLXDProfileUpgradeNotifications returns a NotifyWatcher for observing changes to upgrade series locks.
func (u *LXDProfileUpgradeAPI) WatchLXDProfileUpgradeNotifications(args params.Entities) (params.NotifyWatchResults, error) {
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
		machine, err := u.GetMachine(tag)
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		w, err := machine.WatchLXDProfileUpgradeNotifications()
		if err != nil {
			result.Results[i].Error = ServerError(err)
			continue
		}
		id := u.resources.Register(w)
		result.Results[i].NotifyWatcherId = id
	}
	return result, nil
}

// LXDProfileUpgradeUnitStatus returns the current preparation status of an
// upgrading unit.
// If no series upgrade is in progress an error is returned instead.
func (u *LXDProfileUpgradeAPI) LXDProfileUpgradeUnitStatus(args params.Entities) (params.LXDProfileUpgradeStatusResults, error) {
	u.logger.Tracef("Starting LXDProfileUpgradeUnitStatus with %+v", args)
	return u.unitStatus(args)
}

func (u *LXDProfileUpgradeAPI) GetMachine(tag names.Tag) (LXDProfileUpgradeMachine, error) {
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

func (u *LXDProfileUpgradeAPI) unitStatus(args params.Entities) (params.LXDProfileUpgradeStatusResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.LXDProfileUpgradeStatusResults{}, err
	}

	results := make([]params.LXDProfileUpgradeStatusResult, len(args.Entities))
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
		status, err := unit.LXDProfileUpgradeStatus()
		if err != nil {
			results[i].Error = ServerError(err)
			continue
		}
		results[i].Status = status
	}
	return params.LXDProfileUpgradeStatusResults{Results: results}, nil
}

func (u *LXDProfileUpgradeAPI) getUnit(tag names.Tag) (LXDProfileUpgradeUnit, error) {
	return u.backend.Unit(tag.Id())
}
