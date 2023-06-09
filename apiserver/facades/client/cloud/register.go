// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/domain"
	ccservice "github.com/juju/juju/domain/controllerconfig/service"
	ccstate "github.com/juju/juju/domain/controllerconfig/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Cloud", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV7(ctx) // Do not set error if forcing credential update.
	}, reflect.TypeOf((*CloudAPI)(nil)))
}

// newFacadeV7 is used for API registration.
func newFacadeV7(context facade.Context) (*CloudAPI, error) {
	st := NewStateBackend(context.State())
	ctrlConfigService := ccservice.NewService(
		ccstate.NewState(domain.NewTxnRunnerFactory(context.ControllerDB)),
		domain.NewWatcherFactory(
			context.ControllerDB,
			context.Logger().Child("controllerconfig"),
		),
	)
	pool := NewModelPoolBackend(context.StatePool(), ctrlConfigService)
	systemState, err := pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctlrSt := NewStateBackend(systemState)
	return NewCloudAPI(st, ctlrSt, pool, context.Auth(), context.Logger().Child("cloud"), ctrlConfigService)
}
