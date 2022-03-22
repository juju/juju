// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"reflect"

	"github.com/juju/errors"
	charmscommon "github.com/juju/juju/apiserver/common/charms"
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
	registry.MustRegister("CAASFirewaller", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFacadeLegacy(ctx)
	}, reflect.TypeOf((*Facade)(nil)))

	// TODO(juju3): rename to CAASFirewallerSidecar
	registry.MustRegister("CAASFirewallerEmbedded", 1, func(ctx facade.Context) (facade.Facade, error) {
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
