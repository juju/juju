// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("LeadershipService", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newLeadershipServiceFacade(ctx)
	}, reflect.TypeOf((*LeadershipService)(nil)).Elem())
}

// newLeadershipServiceFacade constructs a new LeadershipService and presents
// a signature that can be used for facade registration.
func newLeadershipServiceFacade(context facade.ModelContext) (LeadershipService, error) {
	claimer, err := context.LeadershipClaimer()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewLeadershipService(claimer, context.Auth())
}
