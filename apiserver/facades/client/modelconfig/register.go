// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ModelConfig", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*ModelConfigAPIV1)(nil)))
	registry.MustRegister("ModelConfig", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*ModelConfigAPIV2)(nil)))
	registry.MustRegister("ModelConfig", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*ModelConfigAPIV3)(nil)))
}

// newFacadeV3 is used for API registration.
func newFacadeV3(ctx facade.Context) (*ModelConfigAPIV3, error) {
	auth := ctx.Auth()

	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewModelConfigAPI(NewStateBackend(model), auth)
}

// newFacadeV2 is used for API registration.
func newFacadeV2(ctx facade.Context) (*ModelConfigAPIV2, error) {
	api, err := newFacadeV3(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ModelConfigAPIV2{api}, nil
}

// newFacadeV1 is used for API registration.
func newFacadeV1(ctx facade.Context) (*ModelConfigAPIV1, error) {
	api, err := newFacadeV2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ModelConfigAPIV1{api}, nil
}
