// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	charmscommon "github.com/juju/juju/apiserver/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	corecharm "github.com/juju/juju/core/charm"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Charms", 5, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newFacadeV5(ctx)
	}, reflect.TypeOf((*APIv5)(nil)))
	registry.MustRegister("Charms", 6, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newFacadeV6(ctx)
	}, reflect.TypeOf((*APIv6)(nil)))
	registry.MustRegister("Charms", 7, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newFacadeV7(ctx)
	}, reflect.TypeOf((*APIv7)(nil)))
}

func newFacadeV7(ctx facade.Context) (*APIv7, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv7{api}, nil
}

func newFacadeV6(ctx facade.Context) (*APIv6, error) {
	api, err := newFacadeV7(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv6{api}, nil
}

func newFacadeV5(ctx facade.Context) (*APIv5, error) {
	api, err := newFacadeV6(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv5{api}, nil
}

// newFacadeBase provides the signature required for facade registration.
func newFacadeBase(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	commonState := &charmscommon.StateShim{st}
	charmInfoAPI, err := charmscommon.NewCharmInfoAPI(commonState, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &API{
		charmInfoAPI:       charmInfoAPI,
		authorizer:         authorizer,
		backendState:       newStateShim(st),
		backendModel:       m,
		charmhubHTTPClient: ctx.HTTPClient(facade.CharmhubHTTPClient),
		newRepoFactory: func(cfg services.CharmRepoFactoryConfig) corecharm.RepositoryFactory {
			return services.NewCharmRepoFactory(cfg)
		},
		tag:             m.ModelTag(),
		requestRecorder: ctx.RequestRecorder(),
		logger:          ctx.Logger().Child("charms"),
	}, nil
}
