// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/permission"
)

// API exposes leadership pinning and unpinning functionality for remote use.
type API interface {
	PinLeadership(params params.PinLeadershipBulkParams) (params.ErrorResults, error)
	UnpinLeadership(params params.PinLeadershipBulkParams) (params.ErrorResults, error)
}

// NewFacade creates and returns a new leadership API.
// This signature is suitable for facade registration.
func NewFacade(ctx facade.Context) (API, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	pinner, err := ctx.LeadershipPinner(model.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewAPI(model.ModelTag(), pinner, ctx.Auth())
}

// NewAPI creates and returns a new leadership API from the input tag,
// Pinner implementation and facade Authorizer.
func NewAPI(modelTag names.ModelTag, pinner leadership.Pinner, authorizer facade.Authorizer) (API, error) {
	return &api{
		modelTag:   modelTag,
		pinner:     pinner,
		authorizer: authorizer,
	}, nil
}

type api struct {
	modelTag   names.ModelTag
	pinner     leadership.Pinner
	authorizer facade.Authorizer
}

// PinLeadership (API) pins the leadership for applications indicated in the
// input arguments.
func (a *api) PinLeadership(param params.PinLeadershipBulkParams) (params.ErrorResults, error) {
	if err := a.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}
	return a.pinOps(param, a.pinner.PinLeadership), nil
}

// PinLeadership (API) unpins the leadership for applications indicated in the
// input arguments.
func (a *api) UnpinLeadership(param params.PinLeadershipBulkParams) (params.ErrorResults, error) {
	if err := a.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}
	return a.pinOps(param, a.pinner.UnpinLeadership), nil
}

func (a *api) pinOps(param params.PinLeadershipBulkParams, op func(string, names.Tag) error) params.ErrorResults {
	results := make([]params.ErrorResult, len(param.Params))
	for i, p := range param.Params {
		appTag, err := names.ParseApplicationTag(p.ApplicationTag)
		if err != nil {
			results[i] = params.ErrorResult{Error: common.ServerError(err)}
			continue
		}
		entityTag, err := names.ParseTag(p.EntityTag)
		if err != nil {
			results[i] = params.ErrorResult{Error: common.ServerError(err)}
			continue
		}
		if err = op(appTag.Id(), entityTag); err != nil {
			results[i] = params.ErrorResult{Error: common.ServerError(err)}
		}
	}
	return params.ErrorResults{Results: results}
}

func (a *api) checkCanWrite() error {
	return a.checkAccess(permission.WriteAccess)
}

func (a *api) checkCanRead() error {
	return a.checkAccess(permission.ReadAccess)
}

func (a *api) checkAccess(access permission.Access) error {
	canAccess, err := a.authorizer.HasPermission(access, a.modelTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canAccess {
		return common.ErrPerm
	}
	return nil
}
