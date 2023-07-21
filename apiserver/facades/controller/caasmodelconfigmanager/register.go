// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"reflect"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	ecservice "github.com/juju/juju/domain/externalcontroller/service"
	ecstate "github.com/juju/juju/domain/externalcontroller/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASModelConfigManager", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newFacade creates a new authorized Facade.
func newFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &Facade{
		auth: authorizer,
		controllerConfigAPI: common.NewControllerConfigAPI(
			ctx.State(),
			ecservice.NewService(
				ecstate.NewState(changestream.NewTxnRunnerFactory(ctx.ControllerDB)),
				domain.NewWatcherFactory(
					ctx.ControllerDB,
					ctx.Logger().Child("caasmodelconfigmanager"),
				),
			),
		),
	}, nil
}
