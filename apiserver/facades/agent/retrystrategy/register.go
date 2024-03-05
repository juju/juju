// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("RetryStrategy", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newRetryStrategyAPI(ctx)
	}, reflect.TypeOf((*RetryStrategyAPI)(nil)))
}

// newRetryStrategyAPI creates a new API endpoint for getting retry strategies.
func newRetryStrategyAPI(ctx facade.ModelContext) (*RetryStrategyAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &RetryStrategyAPI{
		st:    st,
		model: model,
		canAccess: func() (common.AuthFunc, error) {
			return authorizer.AuthOwner, nil
		},
		resources: ctx.Resources(),
	}, nil
}
