// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MigrationFlag", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newFacade wraps New to express the supplied *state.State as a Backend.
func newFacade(ctx facade.ModelContext) (*Facade, error) {
	modelTag := names.NewModelTag(ctx.ModelUUID().String())
	getCanAccessFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if t, ok := tag.(names.ModelTag); ok && t == modelTag {
				return true
			}
			return false
		}, nil
	}
	facade, err := New(
		ctx.WatcherRegistry(),
		ctx.Auth(),
		getCanAccessFunc,
		ctx.DomainServices().ModelMigration(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}
