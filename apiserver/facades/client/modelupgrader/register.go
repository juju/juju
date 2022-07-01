// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/v2/apiserver/common"
	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/juju/v2/environs/context"
	"github.com/juju/juju/v2/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ModelUpgrader", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*ModelUpgraderAPI)(nil)))
}

// newFacadeV1 is used for API registration.
func newFacadeV1(ctx facade.Context) (*ModelUpgraderAPI, error) {
	st := ctx.State()
	pool := ctx.StatePool()
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

	urlGetter := common.NewToolsURLGetter(modelUUID, systemState)
	toolsFinder := common.NewToolsFinder(configGetter, st, urlGetter, newEnviron)

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	apiUser, _ := auth.GetAuthTag().(names.UserTag)
	backend := common.NewUserAwareModelManagerBackend(model, pool, apiUser)
	return NewModelUpgraderAPI(
		systemState.ControllerTag(),
		statePoolShim{StatePool: pool},
		toolsFinder,
		newEnviron,
		common.NewBlockChecker(backend),
		auth,
		context.CallContext(st),
	)
}
