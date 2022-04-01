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
	registry.MustRegister("ModelManager", 9, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV9(ctx) // Adds ValidateModelUpgrade
	}, reflect.TypeOf((*ModelManagerAPI)(nil)))
}

// newFacadeV9 is used for API registration.
func newFacadeV9(ctx facade.Context) (*ModelManagerAPI, error) {
	st := ctx.State()
	pool := ctx.StatePool()
	ctlrSt := pool.SystemState()
	auth := ctx.Auth()

	var err error
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
