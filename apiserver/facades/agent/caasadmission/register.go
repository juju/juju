// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"reflect"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	ccservice "github.com/juju/juju/domain/controllerconfig/service"
	ccstate "github.com/juju/juju/domain/controllerconfig/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASAdmission", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

func newStateFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	ctrlConfigService := ccservice.NewService(
		ccstate.NewState(changestream.NewTxnRunnerFactory(ctx.ControllerDB)),
		domain.NewWatcherFactory(
			ctx.ControllerDB,
			ctx.Logger().Child("controllerconfig"),
		),
	)
	if !authorizer.AuthMachineAgent() {
		return nil, errors.ErrPerm
	}

	return &Facade{
		auth:                authorizer,
		ControllerConfigAPI: common.NewStateControllerConfig(ctx.State(), ctrlConfigService),
	}, nil
}
