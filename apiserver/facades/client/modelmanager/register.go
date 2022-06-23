// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/names/v4"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ModelManager", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*ModelManagerAPIV2)(nil)))
	registry.MustRegister("ModelManager", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*ModelManagerAPIV3)(nil)))
	registry.MustRegister("ModelManager", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*ModelManagerAPIV4)(nil)))
	registry.MustRegister("ModelManager", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV5(ctx) // Adds ChangeModelCredential
	}, reflect.TypeOf((*ModelManagerAPIV5)(nil)))
	registry.MustRegister("ModelManager", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV6(ctx) // Adds cloud specific default config
	}, reflect.TypeOf((*ModelManagerAPIV6)(nil)))
	registry.MustRegister("ModelManager", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV7(ctx) // DestroyModels gains 'force' and max-wait' parameters.
	}, reflect.TypeOf((*ModelManagerAPIV7)(nil)))
	registry.MustRegister("ModelManager", 8, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV8(ctx) // ModelInfo gains credential validity in return.
	}, reflect.TypeOf((*ModelManagerAPIV8)(nil)))
	registry.MustRegister("ModelManager", 9, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV9(ctx) // Adds ValidateModelUpgrade
	}, reflect.TypeOf((*ModelManagerAPIV9)(nil)))
	registry.MustRegister("ModelManager", 10, func(ctx facade.Context) (facade.Facade, error) {
		// ValidateModelUpgrade does target version check and some extra checks for Juju3.
		// Adds UpgradeModel.
		return newFacadeV10(ctx)
	}, reflect.TypeOf((*ModelManagerAPI)(nil)))
}

// newFacadeV10 is used for API registration.
func newFacadeV10(ctx facade.Context) (*ModelManagerAPI, error) {
	st := ctx.State()
	pool := ctx.StatePool()
	ctlrSt, err := pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	auth := ctx.Auth()

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	configGetter := stateenvirons.EnvironConfigGetter{Model: model}

	ctrlModel, err := ctlrSt.Model()
	if err != nil {
		return nil, err
	}

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	apiUser, _ := auth.GetAuthTag().(names.UserTag)

	return NewModelManagerAPI(
		common.NewUserAwareModelManagerBackend(model, pool, apiUser),
		common.NewModelManagerBackend(ctrlModel, pool),
		statePoolShim{
			StatePool: pool,
		},
		configGetter,
		caas.New,
		auth,
		model,
		context.CallContext(st),
	)
}

// newFacadeV9 is used for API registration.
func newFacadeV9(ctx facade.Context) (*ModelManagerAPIV9, error) {
	v10, err := newFacadeV10(ctx)
	if err != nil {
		return nil, err
	}
	return &ModelManagerAPIV9{v10}, nil
}

// newFacadeV8 is used for API registration.
func newFacadeV8(ctx facade.Context) (*ModelManagerAPIV8, error) {
	v9, err := newFacadeV9(ctx)
	if err != nil {
		return nil, err
	}
	return &ModelManagerAPIV8{v9}, nil
}

// newFacadeV7 is used for API registration.
func newFacadeV7(ctx facade.Context) (*ModelManagerAPIV7, error) {
	v8, err := newFacadeV8(ctx)
	if err != nil {
		return nil, err
	}
	return &ModelManagerAPIV7{v8}, nil
}

// newFacadeV6 is used for API registration.
func newFacadeV6(ctx facade.Context) (*ModelManagerAPIV6, error) {
	v7, err := newFacadeV7(ctx)
	if err != nil {
		return nil, err
	}
	return &ModelManagerAPIV6{v7}, nil
}

// newFacadeV5 is used for API registration.
func newFacadeV5(ctx facade.Context) (*ModelManagerAPIV5, error) {
	v6, err := newFacadeV6(ctx)
	if err != nil {
		return nil, err
	}
	return &ModelManagerAPIV5{v6}, nil
}

// newFacadeV4 is used for API registration.
func newFacadeV4(ctx facade.Context) (*ModelManagerAPIV4, error) {
	v5, err := newFacadeV5(ctx)
	if err != nil {
		return nil, err
	}
	return &ModelManagerAPIV4{v5}, nil
}

// newFacadeV3 is used for API registration.
func newFacadeV3(ctx facade.Context) (*ModelManagerAPIV3, error) {
	v4, err := newFacadeV4(ctx)
	if err != nil {
		return nil, err
	}
	return &ModelManagerAPIV3{v4}, nil
}

// newFacadeV2 is used for API registration.
func newFacadeV2(ctx facade.Context) (*ModelManagerAPIV2, error) {
	v3, err := newFacadeV3(ctx)
	if err != nil {
		return nil, err
	}
	return &ModelManagerAPIV2{v3}, nil
}
