// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/permission"
)

// API exposes leadership pinning and unpinning functionality for remote use.
type LeadershipPinningAPI interface {
	PinLeadership(params params.PinLeadershipBulkParams) (params.ErrorResults, error)
	UnpinLeadership(params params.PinLeadershipBulkParams) (params.ErrorResults, error)
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
	return NewLeadershipPinningAPI(model.ModelTag(), pinner, ctx.Auth())
}

// NewLeadershipPinningAPI creates and returns a new leadership API from the
// input tag, Pinner implementation and facade Authorizer.
func NewLeadershipPinningAPI(
	modelTag names.ModelTag, pinner leadership.Pinner, authorizer facade.Authorizer,
) (LeadershipPinningAPI, error) {
	return &leadershipAPI{
		modelTag:   modelTag,
		pinner:     pinner,
		authorizer: authorizer,
	}, nil
}

type leadershipAPI struct {
	modelTag   names.ModelTag
	pinner     leadership.Pinner
	authorizer facade.Authorizer
}

// PinLeadership (API) pins the leadership for applications indicated in the
// input arguments.
func (a *leadershipAPI) PinLeadership(param params.PinLeadershipBulkParams) (params.ErrorResults, error) {
	entity := a.authorizer.GetAuthTag()
	if err := a.checkAccess(entity, permission.WriteAccess); err != nil {
		return params.ErrorResults{}, err
	}

	return a.pinOps(a.pinner.PinLeadership, param, entity), nil
}

// PinLeadership (API) unpins the leadership for applications indicated in the
// input arguments.
func (a *leadershipAPI) UnpinLeadership(param params.PinLeadershipBulkParams) (params.ErrorResults, error) {
	entity := a.authorizer.GetAuthTag()
	if err := a.checkAccess(entity, permission.WriteAccess); err != nil {
		return params.ErrorResults{}, err
	}

	return a.pinOps(a.pinner.UnpinLeadership, param, entity), nil
}

// TODO (manadart 2018-10-19): Querying for pins,
// which will use permission.WriteAccess

func (a *leadershipAPI) pinOps(
	op func(string, names.Tag) error, param params.PinLeadershipBulkParams, entity names.Tag,
) params.ErrorResults {
	results := make([]params.ErrorResult, len(param.Params))
	for i, p := range param.Params {
		appTag, err := names.ParseApplicationTag(p.ApplicationTag)
		if err != nil {
			results[i] = params.ErrorResult{Error: ServerError(err)}
			continue
		}
		if err = op(appTag.Id(), entity); err != nil {
			results[i] = params.ErrorResult{Error: ServerError(err)}
		}
	}
	return params.ErrorResults{Results: results}
}

func (a *leadershipAPI) checkAccess(entity names.Tag, access permission.Access) error {
	switch entity.Kind() {
	case names.MachineTagKind, names.ApplicationTagKind, names.ModelTagKind:
		return nil
	case names.UserTagKind:
		canAccess, err := a.authorizer.HasPermission(access, a.modelTag)
		if err != nil {
			return errors.Trace(err)
		}
		if !canAccess {
			return ErrPerm
		}
		return nil
	default:
		return ErrPerm
	}
}
