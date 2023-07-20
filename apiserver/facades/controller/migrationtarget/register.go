// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	ccservice "github.com/juju/juju/domain/controllerconfig/service"
	ccstate "github.com/juju/juju/domain/controllerconfig/state"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MigrationTarget", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*APIV1)(nil)))
	registry.MustRegister("MigrationTarget", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newFacadeV1 is used for APIV1 registration.
func newFacadeV1(ctx facade.Context) (*APIV1, error) {
	ctrlConfigService := ccservice.NewService(
		ccstate.NewState(changestream.NewTxnRunnerFactory(ctx.ControllerDB)),
		domain.NewWatcherFactory(
			ctx.ControllerDB,
			ctx.Logger().Child("controllerconfig"),
		),
	)
	api, err := NewAPI(
		ctx,
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New),
		ctrlConfigService)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIV1{API: api}, nil
}

// newFacade is used for API registration.
func newFacade(ctx facade.Context) (*API, error) {
	ctrlConfigService := ccservice.NewService(
		ccstate.NewState(changestream.NewTxnRunnerFactory(ctx.ControllerDB)),
		domain.NewWatcherFactory(
			ctx.ControllerDB,
			ctx.Logger().Child("controllerconfig"),
		),
	)
	return NewAPI(
		ctx,
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New),
		ctrlConfigService)
}
