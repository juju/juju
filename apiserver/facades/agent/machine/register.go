// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	ccservice "github.com/juju/juju/domain/controllerconfig/service"
	ccstate "github.com/juju/juju/domain/controllerconfig/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Machiner", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newMachinerAPI(ctx) // Adds RecordAgentHostAndStartTime.
	}, reflect.TypeOf((*MachinerAPI)(nil)))
}

// newMachinerAPI creates a new instance of the Machiner API.
func newMachinerAPI(ctx facade.Context) (*MachinerAPI, error) {
	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctrlConfigService := ccservice.NewService(
		ccstate.NewState(changestream.NewTxnRunnerFactory(ctx.ControllerDB)),
		domain.NewWatcherFactory(
			ctx.ControllerDB,
			ctx.Logger().Child("controllerconfig"),
		),
	)
	return NewMachinerAPIForState(
		systemState,
		ctx.State(),
		ctx.Resources(),
		ctx.Auth(),
		ctrlConfigService,
	)
}
