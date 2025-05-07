// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// LeadershipMachine is an indirection for state.machine.
type LeadershipMachine interface {
	ApplicationNames() ([]string, error)
}

type leadershipMachine struct {
	*state.Machine
}

// LeadershipPinningBackend describes state method wrappers used by this API.
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

// NewLeadershipPinningFromContext creates and returns a new leadership from
// a facade context.
// This signature is suitable for facade registration.
func NewLeadershipPinningFromContext(ctx facade.ModelContext) (*LeadershipPinning, error) {
	st := ctx.State()
	pinner, err := ctx.LeadershipPinner()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelTag := names.NewModelTag(ctx.ModelUUID().String())
	return NewLeadershipPinning(
		leadershipPinningBackend{st},
		modelTag,
		pinner,
		ctx.Auth(),
	)
}

// NewLeadershipPinning creates and returns a new leadership API from the
// input tag, Pinner implementation and facade Authorizer.
func NewLeadershipPinning(
	st LeadershipPinningBackend, modelTag names.ModelTag, pinner leadership.Pinner, authorizer facade.Authorizer,
) (*LeadershipPinning, error) {
	return &LeadershipPinning{
		st:         st,
		modelTag:   modelTag,
		pinner:     pinner,
		authorizer: authorizer,
	}, nil
}

// LeadershipPinning defines a type for pinning and unpinning application
// leaders.
type LeadershipPinning struct {
	st         LeadershipPinningBackend
	modelTag   names.ModelTag
	pinner     leadership.Pinner
	authorizer facade.Authorizer
}

// PinnedLeadership returns all pinned applications and the entities that
// require their pinned behaviour, for leadership in the current model.
func (a *LeadershipPinning) PinnedLeadership(ctx context.Context) (params.PinnedLeadershipResult, error) {
	result := params.PinnedLeadershipResult{}

	err := a.authorizer.HasPermission(ctx, permission.ReadAccess, a.modelTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Result, err = a.pinner.PinnedLeadership()
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// PinApplicationLeaders pins leadership for applications based on the auth
// tag provided.
func (a *LeadershipPinning) PinApplicationLeaders(ctx context.Context) (params.PinApplicationsResults, error) {
	if !a.authorizer.AuthMachineAgent() {
		return params.PinApplicationsResults{}, apiservererrors.ErrPerm
	}

	tag := a.authorizer.GetAuthTag()
	switch tag.Kind() {
	case names.MachineTagKind:
		return a.pinMachineApplications(ctx, tag)
	default:
		return params.PinApplicationsResults{}, apiservererrors.ErrPerm
	}
}

// UnpinApplicationLeaders unpins leadership for applications based on the auth
// tag provided.
func (a *LeadershipPinning) UnpinApplicationLeaders(ctx context.Context) (params.PinApplicationsResults, error) {
	if !a.authorizer.AuthMachineAgent() {
		return params.PinApplicationsResults{}, apiservererrors.ErrPerm
	}

	tag := a.authorizer.GetAuthTag()
	switch tag.Kind() {
	case names.MachineTagKind:
		return a.unpinMachineApplications(ctx, tag)
	default:
		return params.PinApplicationsResults{}, apiservererrors.ErrPerm
	}
}

// GetMachineApplicationNames returns the applications associated with a
// machine.
func (a *LeadershipPinning) GetMachineApplicationNames(ctx context.Context, id string) ([]string, error) {
	m, err := a.st.Machine(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	apps, err := m.ApplicationNames()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apps, nil
}

// PinApplicationLeadersByName takes a slice of application names and attempts
// to pin them accordingly.
func (a *LeadershipPinning) PinApplicationLeadersByName(ctx context.Context, tag names.Tag, appNames []string) (params.PinApplicationsResults, error) {
	return a.pinAppLeadersOps(tag, appNames, a.pinner.PinLeadership)
}

// UnpinApplicationLeadersByName takes a slice of application names and
// attempts to unpin them accordingly.
func (a *LeadershipPinning) UnpinApplicationLeadersByName(ctx context.Context, tag names.Tag, appNames []string) (params.PinApplicationsResults, error) {
	return a.pinAppLeadersOps(tag, appNames, a.pinner.UnpinLeadership)
}

// pinMachineApplications pins leadership for applications represented by units
// running on the auth'd machine.
func (a *LeadershipPinning) pinMachineApplications(ctx context.Context, tag names.Tag) (params.PinApplicationsResults, error) {
	appNames, err := a.GetMachineApplicationNames(ctx, tag.Id())
	if err != nil {
		return params.PinApplicationsResults{}, apiservererrors.ErrPerm
	}
	return a.pinAppLeadersOps(tag, appNames, a.pinner.PinLeadership)
}

// unpinMachineApplications unpins leadership for applications represented by
// units running on the auth'd machine.
func (a *LeadershipPinning) unpinMachineApplications(ctx context.Context, tag names.Tag) (params.PinApplicationsResults, error) {
	appNames, err := a.GetMachineApplicationNames(ctx, tag.Id())
	if err != nil {
		return params.PinApplicationsResults{}, apiservererrors.ErrPerm
	}
	return a.pinAppLeadersOps(tag, appNames, a.pinner.UnpinLeadership)
}

// pinAppLeadersOps runs the input pin/unpin operation against all
// applications entities.
// An assumption is made that the validity of the auth tag has been verified
// by the caller.
func (a *LeadershipPinning) pinAppLeadersOps(tag names.Tag, appNames []string, op func(string, string) error) (params.PinApplicationsResults, error) {
	var result params.PinApplicationsResults

	results := make([]params.PinApplicationResult, len(appNames))
	for i, app := range appNames {
		results[i] = params.PinApplicationResult{
			ApplicationName: app,
		}
		if err := op(app, tag.String()); err != nil {
			results[i].Error = apiservererrors.ServerError(err)
		}
	}
	result.Results = results
	return result, nil
}
