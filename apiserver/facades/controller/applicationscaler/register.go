// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ApplicationScaler", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newAPI(stdCtx, ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}
