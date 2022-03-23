// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/space"
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
	registry.MustRegister("Spaces", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIv2(ctx)
	}, reflect.TypeOf((*APIv2)(nil)))
	registry.MustRegister("Spaces", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIv3(ctx)
	}, reflect.TypeOf((*APIv3)(nil)))
	registry.MustRegister("Spaces", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIv4(ctx)
	}, reflect.TypeOf((*APIv4)(nil)))
	registry.MustRegister("Spaces", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newAPIv5(ctx)
	}, reflect.TypeOf((*APIv5)(nil)))
	registry.MustRegister("Spaces", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newAPIv2 is a wrapper that creates a V2 spaces API.
func newAPIv2(ctx facade.Context) (*APIv2, error) {
	api, err := newAPIv3(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// newAPIv3 is a wrapper that creates a V3 spaces API.
func newAPIv3(ctx facade.Context) (*APIv3, error) {
	api, err := newAPIv4(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{api}, nil
}

// newAPIv4 is a wrapper that creates a V4 spaces API.
func newAPIv4(ctx facade.Context) (*APIv4, error) {
	api, err := newAPIv5(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv4{api}, nil
}

// newAPIv5 is a wrapper that creates a V5 spaces API.
func newAPIv5(ctx facade.Context) (*APIv5, error) {
	api, err := newAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv5{api}, nil
}

// newAPI creates a new Space API server-side facade with a
// state.State backing.
func newAPI(ctx facade.Context) (*API, error) {
	st := ctx.State()
	stateShim, err := NewStateShim(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	check := common.NewBlockChecker(st)
	callContext := context.CallContext(st)

	reloadSpacesEnvirons, err := DefaultReloadSpacesEnvirons(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	auth := ctx.Auth()
	reloadSpacesAuth := DefaultReloadSpacesAuthorizer(auth, check, stateShim)
	reloadSpacesAPI := NewReloadSpacesAPI(
		space.NewState(st),
		reloadSpacesEnvirons,
		EnvironSpacesAdapter{},
		callContext,
		reloadSpacesAuth,
	)

	return newAPIWithBacking(apiConfig{
		ReloadSpacesAPI: reloadSpacesAPI,
		Backing:         stateShim,
		Check:           check,
		Context:         callContext,
		Resources:       ctx.Resources(),
		Authorizer:      auth,
		Factory:         newOpFactory(st),
	})
}
