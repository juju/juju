// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UserManager", 3, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newUserManagerAPI(ctx) // Adds ModelUserInfo
	}, reflect.TypeOf((*UserManagerAPI)(nil)))
}

// newUserManagerAPI provides the signature required for facade registration.
func newUserManagerAPI(ctx facade.Context) (*UserManagerAPI, error) {
	return NewAPI(
		ctx,
		ctx.ServiceFactory().User(),
	)
}
