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
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charm/services"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Charms", 7, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV7(ctx)
	}, reflect.TypeOf((*APIv7)(nil)))
}

func newFacadeV7(ctx facade.ModelContext) (*APIv7, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv7{api}, nil
}

// newFacadeBase provides the signature required for facade registration.
func newFacadeBase(ctx facade.ModelContext) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	commonState := &charmscommon.StateShim{State: st}
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
