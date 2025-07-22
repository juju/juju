// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs/tools"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("AgentTools", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*AgentToolsAPI)(nil)))
}

// newFacade is used to register the facade.
func newFacade(ctx facade.ModelContext) (*AgentToolsAPI, error) {
	st := ctx.State()
	domainServices := ctx.DomainServices()
	return NewAgentToolsAPI(
		st,
		tools.FindTools,
		envVersionUpdate,
		ctx.Auth(),
		ctx.Logger().Child("model"),
		domainServices.Machine(),
		domainServices.Config(),
		domainServices.Agent(),
	)
}
