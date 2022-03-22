// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Registry describes the API facades exposed by some API server.
type Registry interface {
	// MustRegister adds a single named facade at a given version to the
	// registry.
	// Factory will be called when someone wants to instantiate an object of
	// this facade, and facadeType defines the concrete type that the returned
	// object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegister(string, int, facade.Factory, reflect.Type)
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry Registry) {
	registry.MustRegister("RetryStrategy", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newRetryStrategyAPI(ctx)
	}, reflect.TypeOf((*RetryStrategyAPI)(nil)))
}

// newRetryStrategyAPI creates a new API endpoint for getting retry strategies.
func newRetryStrategyAPI(ctx facade.Context) (*RetryStrategyAPI, error) {
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
