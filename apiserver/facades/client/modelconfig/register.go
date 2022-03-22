// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"reflect"

	"github.com/juju/errors"
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
	registry.MustRegister("ModelConfig", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*ModelConfigAPIV1)(nil)))
	registry.MustRegister("ModelConfig", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*ModelConfigAPIV2)(nil)))
}

// newFacadeV2 is used for API registration.
func newFacadeV2(ctx facade.Context) (*ModelConfigAPIV2, error) {
	auth := ctx.Auth()

	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewModelConfigAPI(NewStateBackend(model), auth)
}

// newFacadeV1 is used for API registration.
func newFacadeV1(ctx facade.Context) (*ModelConfigAPIV1, error) {
	api, err := newFacadeV2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ModelConfigAPIV1{api}, nil
}
