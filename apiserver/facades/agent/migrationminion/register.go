// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MigrationMinion", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeFor[*API]())

	registry.MustRegister("MigrationStatusWatcher", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newMigrationStatusWatcher(ctx)
	}, reflect.TypeFor[*MigrationStatusWatcherAPI]())
}

// newFacade provides the signature required for facade registration.
func newFacade(ctx facade.ModelContext) (*API, error) {
	return NewAPI(
		ctx.WatcherRegistry(),
		ctx.Auth(),
		ctx.DomainServices().ModelMigration(),
	)
}

// newMigrationStatusWatcher provides the facade for a migration status watcher.
func newMigrationStatusWatcher(ctx facade.ModelContext) (*MigrationStatusWatcherAPI, error) {
	return NewMigrationStatusWatcherAPI(
		ctx.WatcherRegistry(),
		ctx.Auth(),
		ctx.DomainServices().ModelMigration(),
		ctx.DomainServices().ControllerNode(),
		ctx.DomainServices().ControllerConfig(),
		ctx.ID(),
		ctx.Dispose,
	)
}
