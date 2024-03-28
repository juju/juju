// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ModelManager", 10, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV10(ctx)
	}, reflect.TypeOf((*ModelManagerAPI)(nil)))
	registry.MustRegister("ModelManager", 9, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV9(ctx)
	}, reflect.TypeOf((*ModelManagerAPIV9)(nil)))
}

// newFacadeV9 is used for API registration.
func newFacadeV9(ctx facade.Context) (*ModelManagerAPIV9, error) {
	api, err := newFacadeV10(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ModelManagerAPIV9{api}, nil
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
	modelUUID := model.UUID()

	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	configGetter := stateenvirons.EnvironConfigGetter{Model: model}
	newEnviron := common.EnvironFuncForModel(model, configGetter)

	ctrlModel, err := ctlrSt.Model()
	if err != nil {
		return nil, err
	}
	urlGetter := common.NewToolsURLGetter(modelUUID, systemState)
	toolsFinder := common.NewToolsFinder(configGetter, st, urlGetter, newEnviron)

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	apiUser, _ := auth.GetAuthTag().(names.UserTag)
	backend := common.NewUserAwareModelManagerBackend(model, pool, apiUser)
	return NewModelManagerAPI(
		backend,
		common.NewModelManagerBackend(ctrlModel, pool),
		toolsFinder,
		caas.New,
		common.NewBlockChecker(backend),
		auth,
		model,
		context.CallContext(st),
	)
}
