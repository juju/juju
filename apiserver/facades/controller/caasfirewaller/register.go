// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASFirewaller", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newStateFacade provides the signature required for facade registration.
func newStateFacade(ctx facade.ModelContext) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()

	commonState := &charmscommon.StateShim{ctx.State()}
	commonCharmsAPI, err := charmscommon.NewCharmInfoAPI(commonState, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	appCharmInfoAPI, err := charmscommon.NewApplicationCharmInfoAPI(commonState, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newFacade(
		resources,
		authorizer,
		&stateShim{ctx.State()},
		commonCharmsAPI,
		appCharmInfoAPI,
	)
}
