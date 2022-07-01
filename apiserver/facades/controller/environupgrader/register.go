// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environupgrader

import (
	"reflect"

	"github.com/juju/names/v4"

	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/juju/v2/environs"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("EnvironUpgrader", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newStateFacade provides the signature required for facade registration.
func newStateFacade(ctx facade.Context) (*Facade, error) {
	pool := NewPool(ctx.StatePool())
	registry := environs.GlobalProviderRegistry()
	watcher := common.NewAgentEntityWatcher(
		ctx.State(),
		ctx.Resources(),
		common.AuthFuncForTagKind(names.ModelTagKind),
	)
	statusSetter := common.NewStatusSetter(
		ctx.State(),
		common.AuthFuncForTagKind(names.ModelTagKind),
	)
	return NewFacade(ctx.State(), pool, registry, watcher, statusSetter, ctx.Auth())
}
