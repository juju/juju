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
}

// LXDProfileUpgradeMachine describes machine-receiver state methods.
type LXDProfileUpgradeMachine interface {
	WatchLXDProfileUpgradeNotifications() (state.StringsWatcher, error)
	LXDProfileUpgradeStatus() (model.LXDProfileUpgradeStatus, error)
}

// LXDProfileUpgradeUnit describes unit-receiver state methods.
type LXDProfileUpgradeUnit interface {
	Tag() names.Tag
	AssignedMachineId() (string, error)
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

type lxdProfileMachine struct {
	*state.Machine
}

type LXDProfileUpgradeAPI struct {
	backend   LXDProfileUpgradeBackend
	resources facade.Resources

	logger loggo.Logger

	AccessMachine GetAuthFunc
}

// NewLXDProfileUpgradeAPI returns a new LXDProfileUpgradeAPI. Currently both
// GetAuthFuncs can used to determine current permissions.
func NewLXDProfileUpgradeAPI(
	backend LXDProfileUpgradeBackend,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessMachine GetAuthFunc,
	logger loggo.Logger,
) *LXDProfileUpgradeAPI {
	logger.Tracef("NewLXDProfileUpgradeAPI called with %s", authorizer.GetAuthTag())
	return &LXDProfileUpgradeAPI{
		backend:       backend,
		resources:     resources,
		AccessMachine: accessMachine,
		logger:        logger,
	}
}

// NewExternalLXDProfileUpgradeAPI can be used for API registration.
func NewExternalLXDProfileUpgradeAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessMachine GetAuthFunc,
	logger loggo.Logger,
) *LXDProfileUpgradeAPI {
	return NewLXDProfileUpgradeAPI(LXDProfileUpgradeState{st}, resources, authorizer, accessMachine, logger)
}

// WatchLXDProfileUpgradeNotifications returns a NotifyWatcher for observing changes to upgrade series locks.
func (u *LXDProfileUpgradeAPI) WatchLXDProfileUpgradeNotifications(args params.Entities) (params.NotifyWatchResults, error) {
	u.logger.Tracef("Starting WatchLXDProfileUpgradeNotifications with %+v", args)
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := u.AccessMachine()
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

func (u *LXDProfileUpgradeAPI) GetMachine(tag names.Tag) (LXDProfileUpgradeMachine, error) {
	var id string
	switch tag.Kind() {
	case names.MachineTagKind:
		id = tag.Id()
	default:
	}
	return u.backend.Machine(id)
}
