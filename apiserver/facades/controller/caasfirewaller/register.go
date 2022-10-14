// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"reflect"

	"github.com/juju/errors"

	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASFirewaller", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFacadeLegacy(ctx)
	}, reflect.TypeOf((*Facade)(nil)))

	registry.MustRegister("CAASFirewallerSidecar", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFacadeSidecar(ctx)
	}, reflect.TypeOf((*FacadeSidecar)(nil)))
}

// newStateFacadeLegacy provides the signature required for facade registration.
func newStateFacadeLegacy(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()

	commonState := &charmscommon.StateShim{ctx.State()}
	charmInfoAPI, err := charmscommon.NewCharmInfoAPI(commonState, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	appCharmInfoAPI, err := charmscommon.NewApplicationCharmInfoAPI(commonState, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return newFacadeLegacy(
		resources,
		authorizer,
		&stateShim{ctx.State()},
		charmInfoAPI,
		appCharmInfoAPI,
	)
}

// newStateFacadeSidecar provides the signature required for facade registration.
func newStateFacadeSidecar(ctx facade.Context) (*FacadeSidecar, error) {
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
	return newFacadeSidecar(
		resources,
		authorizer,
		&stateShim{ctx.State()},
		commonCharmsAPI,
		appCharmInfoAPI,
	)
}
