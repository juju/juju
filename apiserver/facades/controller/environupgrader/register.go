// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environupgrader

import (
	"reflect"

	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("EnvironUpgrader", 1, func(ctx facade.Context) (facade.Facade, error) {
		return NewStateFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// NewStateFacade provides the signature required for facade registration.
func NewStateFacade(ctx facade.Context) (*Facade, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}

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
	return NewFacade(ctx.ServiceFactory().Cloud(), pool, registry, watcher, statusSetter)
}
