// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/leadership.go github.com/juju/juju/apiserver/common LeadershipPinningBackend,LeadershipMachine

// LeadershipMachine is an indirection for state.machine.
type LeadershipMachine interface {
	ApplicationNames() ([]string, error)
}

type leadershipMachine struct {
	*state.Machine
}

// LeadershipPinningBacked describes state method wrappers used by this API.
type LeadershipPinningBackend interface {
	Machine(string) (LeadershipMachine, error)
}

type leadershipPinningBackend struct {
	*state.State
}

// Machine wraps state.Machine to return an implementation
// of the LeadershipMachine indirection.
func (s leadershipPinningBackend) Machine(name string) (LeadershipMachine, error) {
	m, err := s.State.Machine(name)
	if err != nil {
		return nil, err
	}
	return leadershipMachine{m}, nil
}

// API exposes leadership pinning and unpinning functionality for remote use.
type LeadershipPinningAPI interface {
	PinMachineApplications() (params.PinApplicationsResults, error)
	UnpinMachineApplications() (params.PinApplicationsResults, error)
	PinnedLeadership() (params.PinnedLeadershipResult, error)
}

// NewLeadershipPinningFacade creates and returns a new leadership API.
// This signature is suitable for facade registration.
func NewLeadershipPinningFacade(ctx facade.Context) (LeadershipPinningAPI, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	pinner, err := ctx.LeadershipPinner(model.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewLeadershipPinningAPI(leadershipPinningBackend{st}, model.ModelTag(), pinner, ctx.Auth())
}

// NewLeadershipPinningAPI creates and returns a new leadership API from the
// input tag, Pinner implementation and facade Authorizer.
func NewLeadershipPinningAPI(
	st LeadershipPinningBackend, modelTag names.ModelTag, pinner leadership.Pinner, authorizer facade.Authorizer,
) (LeadershipPinningAPI, error) {
	return &leadershipPinningAPI{
		st:         st,
		modelTag:   modelTag,
		pinner:     pinner,
		authorizer: authorizer,
	}, nil
}

type leadershipPinningAPI struct {
	st         LeadershipPinningBackend
	modelTag   names.ModelTag
	pinner     leadership.Pinner
	authorizer facade.Authorizer
}

// PinnedLeadership returns all pinned applications and the entities that
// require their pinned behaviour, for leadership in the current model.
func (a *leadershipPinningAPI) PinnedLeadership() (params.PinnedLeadershipResult, error) {
	result := params.PinnedLeadershipResult{}

	canAccess, err := a.authorizer.HasPermission(permission.ReadAccess, a.modelTag)
	if err != nil {
		return result, errors.Trace(err)
	}
	if !canAccess {
		return result, ErrPerm
	}

	result.Result = a.pinner.PinnedLeadership()
	return result, nil
}

// TODO (manadart 2018-10-29): Rename the two methods below (and on the client
// side) to be [Un]PinApplicationLeaders, and derive the list of applications
// based on the authenticating entity.

// PinMachineApplications pins leadership for applications represented by units
// running on the auth'd machine.
func (a *leadershipPinningAPI) PinMachineApplications() (params.PinApplicationsResults, error) {
	if !a.authorizer.AuthMachineAgent() {
		return params.PinApplicationsResults{}, ErrPerm
	}
	return a.pinMachineAppsOps(a.pinner.PinLeadership)
}

// UnpinMachineApplications unpins leadership for applications represented by
// units running on the auth'd machine.
func (a *leadershipPinningAPI) UnpinMachineApplications() (params.PinApplicationsResults, error) {
	if !a.authorizer.AuthMachineAgent() {
		return params.PinApplicationsResults{}, ErrPerm
	}
	return a.pinMachineAppsOps(a.pinner.UnpinLeadership)
}

// pinMachineAppsOps runs the input pin/unpin operation against all
// applications represented by units on the authorised machine.
// An assumption is made that the validity of the auth tag has been verified
// by the caller.
func (a *leadershipPinningAPI) pinMachineAppsOps(op func(string, string) error) (params.PinApplicationsResults, error) {
	result := params.PinApplicationsResults{}

	tag := a.authorizer.GetAuthTag()
	m, err := a.st.Machine(tag.Id())
	if err != nil {
		return result, errors.Trace(err)
	}
	apps, err := m.ApplicationNames()
	if err != nil {
		return result, errors.Trace(err)
	}

	results := make([]params.PinApplicationResult, len(apps))
	for i, app := range apps {
		results[i] = params.PinApplicationResult{
			ApplicationName: app,
		}
		if err := op(app, tag.String()); err != nil {
			results[i].Error = ServerError(err)
		}
	}
	result.Results = results
	return result, nil
}
