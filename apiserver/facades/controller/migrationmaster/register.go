// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v3/apiserver/facade"
	"github.com/juju/juju/v3/migration"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MigrationMaster", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newMigrationMasterFacade(ctx) // Adds MinionReportTimeout.
	}, reflect.TypeOf((*API)(nil)))
}

// newMigrationMasterFacade exists to provide the required signature for API
// registration, converting st to backend.
func newMigrationMasterFacade(ctx facade.Context) (*API, error) {
	controllerState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	precheckBackend, err := migration.PrecheckShim(ctx.State(), controllerState)
	if err != nil {
		return nil, errors.Annotate(err, "creating precheck backend")
	}
	return NewAPI(
		newBacked(ctx.State()),
		precheckBackend,
		migration.PoolShim(ctx.StatePool()),
		ctx.Resources(),
		ctx.Auth(),
		ctx.Presence(),
	)
}
